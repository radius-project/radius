/*
Copyright 2024 The Radius Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package reconciler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/filesystem"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	crconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/yaml"
)

type fluxWriteCounter struct {
	k8sclient.Client
	creates               int
	updates               int
	deletes               int
	activeTemplateUpdates int
}

func (c *fluxWriteCounter) Create(ctx context.Context, object k8sclient.Object, opts ...k8sclient.CreateOption) error {
	c.creates++
	return c.Client.Create(ctx, object, opts...)
}

func (c *fluxWriteCounter) Update(ctx context.Context, object k8sclient.Object, opts ...k8sclient.UpdateOption) error {
	c.updates++
	if _, ok := object.(*radappiov1alpha3.DeploymentTemplate); ok && object.GetDeletionTimestamp().IsZero() {
		c.activeTemplateUpdates++
	}
	return c.Client.Update(ctx, object, opts...)
}

func (c *fluxWriteCounter) Delete(ctx context.Context, object k8sclient.Object, opts ...k8sclient.DeleteOption) error {
	c.deletes++
	return c.Client.Delete(ctx, object, opts...)
}

func fluxTemplateCRD(t *testing.T) *apiextensionsv1.CustomResourceDefinition {
	t.Helper()
	data, err := os.ReadFile("../../../deploy/Chart/crds/radius/radapp.io_deploymenttemplates.yaml")
	require.NoError(t, err)
	var crd apiextensionsv1.CustomResourceDefinition
	require.NoError(t, yaml.Unmarshal(data, &crd))
	return &crd
}

func Test_FluxController_Preflight(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name       string
		config     string
		unselected bool
		conflict   bool
		wantErr    string
	}{
		{name: "unselected source", unselected: true},
		{name: "existing different owner", conflict: true, config: "config:\n- name: app.bicep\n  namespace: target\n"},
		{name: "duplicate targets", config: "config:\n- name: app.bicep\n- name: app.bicep\n", wantErr: "duplicate"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			source := readyFluxSource()
			objects := []k8sclient.Object{source, fluxTemplateCRD(t)}
			if tt.conflict {
				objects = append(objects, &radappiov1alpha3.DeploymentTemplate{
					ObjectMeta: metav1.ObjectMeta{Name: "app.bicep", Namespace: "target", UID: "existing"},
					Spec:       radappiov1alpha3.DeploymentTemplateSpec{Template: "unchanged", Repository: "other"},
				})
			}
			c := &fluxWriteCounter{Client: fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).WithObjects(objects...).Build()}
			options := hostoptions.FluxOptions{}
			if !tt.unselected {
				options = hostoptions.FluxOptions{Enabled: true, Sources: []hostoptions.FluxSourceSelection{{
					Namespace: source.Namespace, Name: source.Name, UID: string(source.UID),
				}}}
			}
			policy, err := NewFluxSourcePolicy(options, c)
			require.NoError(t, err)
			mocks := gomock.NewController(t)
			fs := filesystem.NewMemMapFileSystem()
			fetcher := NewMockArchiveFetcher(mocks)
			compiler := bicep.NewMockInterface(mocks)
			if !tt.unselected {
				fetcher.EXPECT().Fetch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_, _, dir string) error {
					require.NoError(t, fs.WriteFile(path.Join(dir, radiusConfigFileName), []byte(tt.config), 0o600))
					return fs.WriteFile(path.Join(dir, "app.bicep"), []byte(""), 0o600)
				})
			}
			r := &FluxController{Client: c, SourcePolicy: policy, FileSystem: fs, ArchiveFetcher: fetcher, Bicep: compiler}
			result, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: k8sclient.ObjectKeyFromObject(source)})
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			if tt.conflict {
				require.Positive(t, result.RequeueAfter)
			}
			require.Zero(t, c.creates)
			require.Zero(t, c.updates)
			require.Zero(t, c.deletes)
		})
	}
}

func Test_FluxController_PruneSourceIdentity(t *testing.T) {
	t.Parallel()
	source := readyFluxSource()
	ref := sourceReference(source)
	otherRef := ref.DeepCopy()
	otherRef.Namespace, otherRef.UID = "another-source-namespace", "another-uid"
	owned := &radappiov1alpha3.DeploymentTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "old.bicep", Namespace: "target", UID: "owned"},
		Spec:       radappiov1alpha3.DeploymentTemplateSpec{Repository: source.Name, SourceRef: ref},
	}
	unrelated := owned.DeepCopy()
	unrelated.Namespace, unrelated.UID, unrelated.Spec.SourceRef = "other-target", "unrelated", otherRef
	pending := owned.DeepCopy()
	pending.Name, pending.UID = "pending.bicep", "pending"
	pending.DeletionTimestamp = new(metav1.Now())
	pending.Finalizers = []string{DeploymentTemplateFinalizer}
	c := &fluxWriteCounter{Client: fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).
		WithIndex(&radappiov1alpha3.DeploymentTemplate{}, deploymentTemplateSourceField, deploymentTemplateSourceIndexer).
		WithObjects(source, owned, pending, unrelated, fluxTemplateCRD(t)).Build()}
	policy, err := NewFluxSourcePolicy(hostoptions.FluxOptions{Enabled: true, Sources: []hostoptions.FluxSourceSelection{{
		Namespace: source.Namespace, Name: source.Name, UID: string(source.UID),
	}}}, c)
	require.NoError(t, err)
	mocks := gomock.NewController(t)
	fs := filesystem.NewMemMapFileSystem()
	fetcher := NewMockArchiveFetcher(mocks)
	fetcher.EXPECT().Fetch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_, _, dir string) error {
		return fs.WriteFile(path.Join(dir, radiusConfigFileName), []byte("config: []\n"), 0o600)
	})
	r := &FluxController{Client: c, SourcePolicy: policy, FileSystem: fs, ArchiveFetcher: fetcher, Bicep: bicep.NewMockInterface(mocks)}
	_, err = r.Reconcile(t.Context(), ctrl.Request{NamespacedName: k8sclient.ObjectKeyFromObject(source)})
	require.NoError(t, err)
	require.Equal(t, 1, c.deletes)
	require.True(t, apierrors.IsNotFound(c.Get(t.Context(), k8sclient.ObjectKeyFromObject(owned), &radappiov1alpha3.DeploymentTemplate{})))
	var got radappiov1alpha3.DeploymentTemplate
	require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(unrelated), &got))
	require.Equal(t, unrelated.Spec, got.Spec)
	require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(pending), &radappiov1alpha3.DeploymentTemplate{}),
		"pending cleanup must not prevent pruning other removed targets")
}

func Test_FluxController_NamespaceMove(t *testing.T) {
	t.Parallel()
	for _, deleting := range []bool{false, true} {
		name := "existing target"
		if deleting {
			name = "target still draining"
		}
		t.Run(name, func(t *testing.T) {
			source := readyFluxSource()
			provider, err := sdkclients.NewDefaultProviderConfig("shared").String()
			require.NoError(t, err)
			previous := &radappiov1alpha3.DeploymentTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "app.bicep", Namespace: "old-target", UID: "template-uid",
					Finalizers: []string{DeploymentTemplateFinalizer}},
				Spec: radappiov1alpha3.DeploymentTemplateSpec{
					Repository: source.Name, SourceRef: sourceReference(source), ProviderConfig: provider, Template: "{}",
				},
			}
			if deleting {
				previous.DeletionTimestamp = new(metav1.Now())
			}
			c := &fluxWriteCounter{Client: fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).
				WithIndex(&radappiov1alpha3.DeploymentTemplate{}, deploymentTemplateSourceField, deploymentTemplateSourceIndexer).
				WithObjects(source, previous, fluxTemplateCRD(t)).Build()}
			var before radappiov1alpha3.DeploymentTemplate
			require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(previous), &before))
			policy := selectedSourcePolicy(t, c, source)
			mocks := gomock.NewController(t)
			fs := filesystem.NewMemMapFileSystem()
			fetcher := NewMockArchiveFetcher(mocks)
			fetcher.EXPECT().Fetch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_, _, dir string) error {
				require.NoError(t, fs.WriteFile(path.Join(dir, "app.bicep"), nil, 0o600))
				return fs.WriteFile(path.Join(dir, radiusConfigFileName),
					[]byte("config:\n- name: app.bicep\n  namespace: new-target\n  resourceGroup: shared\n"), 0o600)
			})
			r := &FluxController{Client: c, SourcePolicy: policy, FileSystem: fs,
				ArchiveFetcher: fetcher, Bicep: bicep.NewMockInterface(mocks)}
			result, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: k8sclient.ObjectKeyFromObject(source)})
			require.NoError(t, err)
			require.Positive(t, result.RequeueAfter)
			require.Zero(t, c.creates)
			require.Zero(t, c.updates)
			require.Zero(t, c.deletes)
			var got radappiov1alpha3.DeploymentTemplate
			require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(previous), &got))
			require.Equal(t, previous.Spec, got.Spec)
			require.Equal(t, previous.UID, got.UID)
			require.Equal(t, before.DeletionTimestamp, got.DeletionTimestamp)
		})
	}
}

func Test_FluxController_PruneAfterSourceGeneration(t *testing.T) {
	t.Parallel()
	for _, alreadyDeleting := range []bool{false, true} {
		name := "new deletion"
		if alreadyDeleting {
			name = "resume old generation deletion"
		}
		t.Run(name, func(t *testing.T) {
			source := readyFluxSource()
			oldRef := sourceReference(source)
			source.Generation = 2
			source.Status.ObservedGeneration = 2
			source.Status.Conditions[0].ObservedGeneration = 2
			provider, err := sdkclients.NewDefaultProviderConfig("target").String()
			require.NoError(t, err)
			previous := &radappiov1alpha3.DeploymentTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "old.bicep", Namespace: "target", UID: "template-uid",
					Finalizers: []string{DeploymentTemplateFinalizer}},
				Spec: radappiov1alpha3.DeploymentTemplateSpec{Template: "{}", Repository: source.Name, SourceRef: oldRef, ProviderConfig: provider},
			}
			if alreadyDeleting {
				previous.DeletionTimestamp = new(metav1.Now())
			}
			child := &radappiov1alpha3.DeploymentResource{
				ObjectMeta: metav1.ObjectMeta{Name: "output", Namespace: previous.Namespace, UID: "output-uid",
					Finalizers: []string{DeploymentResourceFinalizer},
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: radappiov1alpha3.GroupVersion.String(), Kind: deploymentTemplateKind,
						Name: previous.Name, UID: previous.UID, Controller: new(true),
					}}},
				Spec: radappiov1alpha3.DeploymentResourceSpec{Id: "/planes/radius/local/resourceGroups/target/providers/Applications.Core/containers/output"},
			}
			c := &fluxWriteCounter{Client: fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).
				WithStatusSubresource(&radappiov1alpha3.DeploymentTemplate{}, &radappiov1alpha3.DeploymentResource{}).
				WithIndex(&radappiov1alpha3.DeploymentTemplate{}, deploymentTemplateSourceField, deploymentTemplateSourceIndexer).
				WithObjects(source, previous, child, fluxTemplateCRD(t)).Build()}
			policy := selectedSourcePolicy(t, c, source)
			mocks := gomock.NewController(t)
			fs := filesystem.NewMemMapFileSystem()
			fetcher := NewMockArchiveFetcher(mocks)
			fetcher.EXPECT().Fetch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_, _, dir string) error {
				return fs.WriteFile(path.Join(dir, radiusConfigFileName), []byte("config: []\n"), 0o600)
			})
			r := &FluxController{Client: c, SourcePolicy: policy, FileSystem: fs,
				ArchiveFetcher: fetcher, Bicep: bicep.NewMockInterface(mocks)}
			_, err = r.Reconcile(t.Context(), ctrl.Request{NamespacedName: k8sclient.ObjectKeyFromObject(source)})
			require.NoError(t, err)
			var deleting radappiov1alpha3.DeploymentTemplate
			require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(previous), &deleting))
			require.NotNil(t, deleting.DeletionTimestamp)
			require.Equal(t, source.Generation, deleting.Spec.SourceRef.Generation)
			require.Equal(t, previous.UID, deleting.UID)
			require.Equal(t, previous.Spec.Template, deleting.Spec.Template)
			require.Zero(t, c.activeTemplateUpdates, "cleanup authorization must not enable old content before deletion")
			mockBackend := sdkclients.NewMockResourceDeploymentsClient()
			backend := &sourceTestDeployments{ResourceDeploymentsClient: mockBackend}
			templateController := &DeploymentTemplateReconciler{Client: c, SourcePolicy: policy, EventRecorder: record.NewFakeRecorder(5),
				ResourceDeploymentsClient: backend}
			_, err = templateController.Reconcile(t.Context(), ctrl.Request{NamespacedName: k8sclient.ObjectKeyFromObject(previous)})
			require.ErrorContains(t, err, "owned resources")
			resourceController := &DeploymentResourceReconciler{Client: c, SourcePolicy: policy, EventRecorder: record.NewFakeRecorder(5),
				ResourceDeploymentsClient: backend}
			var pending radappiov1alpha3.DeploymentResource
			require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(child), &pending))
			eligible, err := resourceController.deleteIsEligible(t.Context(), &pending, metav1.GetControllerOf(&pending))
			require.NoError(t, err)
			require.True(t, eligible, "current-generation cleanup must retain the original owner and scope: %+v", pending.OwnerReferences)
			_, err = resourceController.Reconcile(t.Context(), ctrl.Request{NamespacedName: k8sclient.ObjectKeyFromObject(child)})
			require.NoError(t, err)
			require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(child), &pending))
			require.NotNil(t, pending.Status.Operation)
			require.Equal(t, 1, backend.deletes)
			require.Zero(t, backend.creates)
			mockBackend.CompleteOperation(pending.Status.Operation.ResumeToken, nil)
			_, err = resourceController.Reconcile(t.Context(), ctrl.Request{NamespacedName: k8sclient.ObjectKeyFromObject(child)})
			require.NoError(t, k8sclient.IgnoreNotFound(err))
			require.True(t, apierrors.IsNotFound(c.Get(t.Context(), k8sclient.ObjectKeyFromObject(child), &radappiov1alpha3.DeploymentResource{})))
			require.Equal(t, 1, backend.deletes)
			_, err = templateController.Reconcile(t.Context(), ctrl.Request{NamespacedName: k8sclient.ObjectKeyFromObject(previous)})
			require.NoError(t, err)
			require.True(t, apierrors.IsNotFound(c.Get(t.Context(), k8sclient.ObjectKeyFromObject(previous), &radappiov1alpha3.DeploymentTemplate{})))
			require.Zero(t, backend.creates)
		})
	}
}

func Test_FluxController_PruneInactiveSource(t *testing.T) {
	t.Parallel()
	source := readyFluxSource()
	ref := sourceReference(source)
	source.Generation = 2
	source.Spec.Suspend = true
	previous := &radappiov1alpha3.DeploymentTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "old.bicep", Namespace: "target", UID: "template-uid",
			DeletionTimestamp: new(metav1.Now()), Finalizers: []string{DeploymentTemplateFinalizer}},
		Spec: radappiov1alpha3.DeploymentTemplateSpec{Repository: source.Name, SourceRef: ref},
	}
	c := &fluxWriteCounter{Client: fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).WithObjects(source, previous).Build()}
	r := &FluxController{Client: c, SourcePolicy: selectedSourcePolicy(t, c, source)}
	ref.Generation = source.Generation
	err := r.pruneTemplate(t.Context(), k8sclient.ObjectKeyFromObject(previous), ref)
	require.ErrorContains(t, err, "SourceNotReady")
	require.Zero(t, c.updates)
	require.Zero(t, c.deletes)
	r.SourcePolicy, err = NewFluxSourcePolicy(hostoptions.FluxOptions{}, c)
	require.NoError(t, err)
	require.ErrorContains(t, r.pruneTemplate(t.Context(), k8sclient.ObjectKeyFromObject(previous), ref), "SourceNotSelected")
	require.Zero(t, c.updates)
	require.Zero(t, c.deletes)
}

func Test_FluxController_DistinctScopeTarget(t *testing.T) {
	t.Parallel()
	source := readyFluxSource()
	ref := sourceReference(source)
	provider, err := sdkclients.NewDefaultProviderConfig("old-group").String()
	require.NoError(t, err)
	previous := &radappiov1alpha3.DeploymentTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "app.bicep", Namespace: "old-target"},
		Spec:       radappiov1alpha3.DeploymentTemplateSpec{Repository: source.Name, SourceRef: ref, ProviderConfig: provider},
	}
	c := fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).WithObjects(source, previous).Build()
	r := &FluxController{Client: c, SourcePolicy: selectedSourcePolicy(t, c, source)}
	entries, err := r.preflightTargets(t.Context(), []ConfigEntry{{Name: previous.Name, Namespace: "new-target", ResourceGroup: "new-group"}}, ref)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "new-target", entries[0].Namespace)
	require.Equal(t, "new-group", entries[0].ResourceGroup)
}

func Test_FluxController_ProtectedWriters(t *testing.T) {
	SkipWithoutEnvironment(t)
	ctx := t.Context()
	admin, err := k8sclient.New(config, k8sclient.Options{Scheme: scheme})
	require.NoError(t, err)
	const namespace = "flux-protected-writers"
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	require.NoError(t, admin.Create(ctx, ns))
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second*10)
		defer cancel()
		require.NoError(t, k8sclient.IgnoreNotFound(admin.Delete(cleanupCtx, ns)))
	})
	source := makeGitRepository(types.NamespacedName{Namespace: namespace, Name: "source"}, "https://example.invalid/fixture.git")
	require.NoError(t, admin.Create(ctx, &source))
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "reader", Namespace: namespace},
		Rules:      []rbacv1.PolicyRule{{APIGroups: []string{sourcev1.GroupVersion.Group}, Resources: []string{"gitrepositories"}, Verbs: []string{"get"}}},
	}
	require.NoError(t, admin.Create(ctx, role))
	require.NoError(t, admin.Create(ctx, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "reader", Namespace: namespace},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: role.Name},
		Subjects:   []rbacv1.Subject{{APIGroup: rbacv1.GroupName, Kind: "User", Name: "flux-fixture-reader"}},
	}))
	userConfig := rest.CopyConfig(config)
	userConfig.Impersonate = rest.ImpersonationConfig{UserName: "flux-fixture-reader", Groups: []string{"system:authenticated"}}
	reader, err := k8sclient.New(userConfig, k8sclient.Options{Scheme: scheme})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return reader.Get(ctx, k8sclient.ObjectKeyFromObject(&source), &sourcev1.GitRepository{}) == nil
	}, time.Second*10, time.Millisecond*100, "read-only principal must be authenticated and permitted to read its source")
	source.Spec.URL = "https://example.invalid/changed.git"
	require.True(t, apierrors.IsForbidden(reader.Update(ctx, &source)))
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "verification", Namespace: namespace}}
	require.NoError(t, admin.Create(ctx, secret))
	secret.Data = map[string][]byte{"public-key": []byte("fixture-public-material")}
	require.True(t, apierrors.IsForbidden(reader.Update(ctx, secret)))
	dt := &radappiov1alpha3.DeploymentTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "application", Namespace: namespace},
		Spec: radappiov1alpha3.DeploymentTemplateSpec{Repository: source.Name, SourceRef: &radappiov1alpha3.DeploymentTemplateSourceReference{
			Namespace: namespace, Name: source.Name, UID: source.UID, Generation: source.Generation,
		}},
	}
	require.NoError(t, admin.Create(ctx, dt))
	dt.Spec.Repository, dt.Spec.SourceRef = "", nil
	require.True(t, apierrors.IsForbidden(reader.Update(ctx, dt)), "clearing attribution must not create a direct path")
	require.True(t, apierrors.IsForbidden(reader.Status().Update(ctx, dt)))
	require.True(t, apierrors.IsForbidden(reader.Delete(ctx, dt)))
	require.True(t, apierrors.IsForbidden(reader.Create(ctx, &radappiov1alpha3.DeploymentTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "direct", Namespace: namespace},
	})))
}

func Test_FluxController_Basic(t *testing.T) {
	testGitRepoName := "flux-basic-repo"
	testGitRepoURL := fmt.Sprintf("https://github.com/radius-project/%s.git", testGitRepoName)
	testGitRepoSHA := "sha256:1234"

	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	setupOpts := setupFluxControllerTestOptions{
		testGitRepoName,
		testGitRepoURL,
		testGitRepoSHA,
		mctrl,
	}

	steps := []Step{
		{
			Path: "testdata/flux-basic",
		},
	}

	runOpts := setupFluxControllerTest(t, setupOpts, steps)

	runFluxControllerTest(t, runOpts, steps)
}

func Test_FluxController_Update(t *testing.T) {
	testGitRepoName := "flux-update-repo"
	testGitRepoURL := fmt.Sprintf("https://github.com/radius-project/%s.git", testGitRepoName)
	testGitRepoSHA := "sha256:1234"

	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	setupOpts := setupFluxControllerTestOptions{
		testGitRepoName,
		testGitRepoURL,
		testGitRepoSHA,
		mctrl,
	}

	steps := []Step{
		{
			Path: "testdata/flux-update/step-1",
		},
		{
			Path: "testdata/flux-update/step-2",
		},
	}

	runOpts := setupFluxControllerTest(t, setupOpts, steps)

	runFluxControllerTest(t, runOpts, steps)
}

type Step struct {
	Path            string
	BicepFiles      []string
	BicepParamFiles []string
}

type setupFluxControllerTestOptions struct {
	testGitRepoName string
	testGitRepoURL  string
	testGitRepoSHA  string

	mctrl *gomock.Controller
}

type runFluxControllerTestOptions struct {
	testGitRepoName string
	testGitRepoURL  string
	testGitRepoSHA  string

	archiveFetcher *MockArchiveFetcher
	filesystem     *filesystem.MemMapFileSystem
	bicep          *bicep.MockInterface
	client         k8sclient.Client
}

func setupFluxControllerTest(t *testing.T, opts setupFluxControllerTestOptions, steps []Step) runFluxControllerTestOptions {
	SkipWithoutEnvironment(t)

	// Shut down the manager when the test exits.
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	fs := filesystem.NewMemMapFileSystem()
	archiveFetcher := NewMockArchiveFetcher(opts.mctrl)
	bicep := bicep.NewMockInterface(opts.mctrl)

	direct, err := k8sclient.New(config, k8sclient.Options{Scheme: scheme})
	require.NoError(t, err)
	selected := make(map[types.NamespacedName]bool)
	sourceOptions := hostoptions.FluxOptions{Enabled: true}
	for _, step := range steps {
		radiusConfig, err := ParseRadiusGitOpsConfig(path.Join(step.Path, "radius-gitops-config.yaml"))
		require.NoError(t, err)
		for _, entry := range radiusConfig.Config {
			namespace := entry.Namespace
			if namespace == "" {
				namespace = strings.TrimSuffix(entry.Name, path.Ext(entry.Name))
			}
			key := types.NamespacedName{Namespace: namespace, Name: opts.testGitRepoName}
			if selected[key] {
				continue
			}
			selected[key] = true
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			err := direct.Create(ctx, ns)
			if err != nil {
				require.NoError(t, k8sclient.IgnoreAlreadyExists(err))
			}
			source := makeGitRepository(key, opts.testGitRepoURL)
			require.NoError(t, direct.Create(ctx, &source))
			sourceOptions.Sources = append(sourceOptions.Sources, hostoptions.FluxSourceSelection{
				Namespace: namespace, Name: source.Name, UID: string(source.UID),
			})
			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
				defer cleanupCancel()
				require.NoError(t, k8sclient.IgnoreNotFound(direct.Delete(cleanupCtx, &source)))
			})
		}
	}

	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme: scheme,
		Controller: crconfig.Controller{
			SkipNameValidation: new(true),
		},
		Metrics: server.Options{
			BindAddress: "0",
		},
		LeaderElection: false,
	})
	require.NoError(t, err)
	sourcePolicy, err := NewFluxSourcePolicy(sourceOptions, mgr.GetAPIReader())
	require.NoError(t, err)

	fluxController := &FluxController{
		Client:         mgr.GetClient(),
		FileSystem:     fs,
		Bicep:          bicep,
		ArchiveFetcher: archiveFetcher,
		SourcePolicy:   sourcePolicy,
	}
	err = (fluxController).SetupWithManager(mgr)
	require.NoError(t, err)

	mgrCtx, mgrCancel := context.WithCancel(ctx)

	managerErr := make(chan error, 1)
	managerStopped := make(chan struct{})
	go func() {
		defer close(managerStopped)
		t.Log("Starting manager...")
		if err := mgr.Start(mgrCtx); err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				managerErr <- fmt.Errorf("manager failed to start: %w", err)
			}
		}
		t.Log("Manager stopped.")
	}()

	// Cancel the manager context and wait for the manager goroutine to fully exit before the test
	// completes, so it does not leak into subsequent tests or into package teardown. The wait is
	// bounded so a manager that fails to stop fails the test instead of hanging until the global
	// `go test` timeout.
	t.Cleanup(func() {
		mgrCancel()
		waitForManagerShutdown(t, managerStopped)
	})

	t.Log("Waiting for cache to be ready")
	waitCtx, waitCancel := context.WithTimeout(ctx, 30*time.Second)
	defer waitCancel()

	err = wait.PollUntilContextTimeout(waitCtx, 100*time.Millisecond, 30*time.Second, true,
		func(pollCtx context.Context) (bool, error) {
			select {
			case err := <-managerErr:
				return false, fmt.Errorf("manager failed to start: %w", err)
			default:
				// No immediate error, check cache
			}

			if mgr.GetCache().WaitForCacheSync(pollCtx) {
				return true, nil
			}
			return false, nil
		})

	require.NoError(t, err, "timeout or error waiting for cache to be ready")
	t.Log("Cache is ready")

	var archiveFetcherCalls []any
	var bicepCalls []any
	for _, step := range steps {
		s := step // capture loop variable
		call := archiveFetcher.EXPECT().
			Fetch(opts.testGitRepoURL, opts.testGitRepoSHA, gomock.Any()).
			Return(nil).
			Times(1).
			Do(func(archiveURL, digest, dir string) {
				err := filepath.WalkDir(s.Path, func(srcPath string, info os.DirEntry, err error) error {
					if err != nil {
						return err
					}
					relPath, err := filepath.Rel(s.Path, srcPath)
					if err != nil {
						return err
					}
					dstPath := filepath.Join(dir, relPath)
					if info.IsDir() {
						return fs.MkdirAll(dstPath, 0755)
					}
					// Read contents of srcFile and write to destination.
					data, err := os.ReadFile(srcPath)
					if err != nil {
						return err
					}
					return fs.WriteFile(dstPath, data, 0644)
				})
				require.NoError(t, err)
			})

		archiveFetcherCalls = append(archiveFetcherCalls, call)

		radiusConfig, err := ParseRadiusGitOpsConfig(path.Join(s.Path, "radius-gitops-config.yaml"))
		require.NoError(t, err)
		require.NotNil(t, radiusConfig)

		for _, configEntry := range radiusConfig.Config {
			ce := configEntry // capture loop variable
			name := ce.Name
			nameBase := strings.TrimSuffix(name, path.Ext(name))
			outFile := fmt.Sprintf("%s.json", nameBase)

			bicepBuildCall := bicep.EXPECT().
				Call("build", gomock.Cond(func(s string) bool { return strings.HasSuffix(s, ce.Name) }), "--outfile", gomock.Cond(func(s string) bool { return strings.HasSuffix(s, outFile) })).
				Return(nil, nil).
				Times(1).
				Do(func(args ...string) {
					filePath := args[1]
					outFilePath := args[3]
					outFileName := filepath.Base(outFilePath)
					localFilePath := s.Path
					fileContent, err := os.ReadFile(path.Join(localFilePath, outFileName))
					require.NoError(t, err)
					err = fs.WriteFile(filePath, fileContent, 0644)
					require.NoError(t, err)
				})
			bicepCalls = append(bicepCalls, bicepBuildCall)

			if ce.Params != "" {
				bicepBuildParamsCall := bicep.EXPECT().
					Call("build-params", gomock.Any(), "--outfile", gomock.Any()).
					Return(nil, nil).
					Times(1).
					Do(func(args ...string) {
						filePath := args[1]
						outFilePath := args[3]
						outFileName := filepath.Base(outFilePath)
						localFilePath := s.Path
						fileContent, err := os.ReadFile(path.Join(localFilePath, outFileName))
						require.NoError(t, err)
						err = fs.WriteFile(filePath, fileContent, 0644)
						require.NoError(t, err)
					})
				bicepCalls = append(bicepCalls, bicepBuildParamsCall)
			}
		}
	}
	gomock.InOrder(archiveFetcherCalls...)
	gomock.InOrder(bicepCalls...)

	return runFluxControllerTestOptions{
		testGitRepoName: opts.testGitRepoName,
		testGitRepoURL:  opts.testGitRepoURL,
		testGitRepoSHA:  opts.testGitRepoSHA,

		archiveFetcher: archiveFetcher,
		filesystem:     fs,
		bicep:          bicep,
		client:         mgr.GetClient(),
	}
}

func runFluxControllerTest(t *testing.T, opts runFluxControllerTestOptions, steps []Step) {
	ctx := t.Context()

	// Track namespaces created during the test for cleanup
	namespacesToCleanup := make(map[string]bool)
	defer func() {
		// Clean up namespaces at the end of the test using fresh context
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second) //nolint:usetesting
		defer cancel()

		for ns := range namespacesToCleanup {
			t.Logf("Cleaning up namespace: %s", ns)
			namespace := &corev1.Namespace{
				Name: ns,
			}
			err := opts.client.Delete(cleanupCtx, namespace)
			if err != nil && k8sclient.IgnoreNotFound(err) != nil {
				t.Logf("Failed to delete namespace %s: %v", ns, err)
			}
		}
	}()

	for stepIndex, step := range steps {
		stepNumber := stepIndex + 1
		radiusConfig, err := ParseRadiusGitOpsConfig(path.Join(step.Path, "radius-gitops-config.yaml"))
		require.NoError(t, err)
		require.NotNil(t, radiusConfig)

		for _, configEntry := range radiusConfig.Config {
			name := configEntry.Name
			nameBase := strings.TrimSuffix(name, path.Ext(name))
			namespaceName := configEntry.Namespace
			if namespaceName == "" {
				namespaceName = nameBase
			}

			// Track namespace for cleanup
			namespacesToCleanup[namespaceName] = true

			// Create a namespace if it does not exist
			if err := opts.client.Get(ctx, types.NamespacedName{Name: namespaceName}, &corev1.Namespace{}); err != nil {
				if k8sclient.IgnoreNotFound(err) != nil {
					require.NoError(t, err)
				}
				namespace := &corev1.Namespace{
					Name: namespaceName,
				}
				err := opts.client.Create(ctx, namespace)
				require.NoError(t, err)
			}

			gitRepo := sourcev1.GitRepository{}
			gitRepoNamespacedName := types.NamespacedName{Name: opts.testGitRepoName, Namespace: namespaceName}
			if stepNumber == 1 {
				// Sources were created before manager startup so selection uses their actual UIDs.
				err = waitForGitRepositoryToExistWithGeneration(ctx, opts.client, gitRepoNamespacedName, &gitRepo, int64(stepNumber))
				require.NoError(t, err, "GitRepository was not created successfully")
			}

			// Fetch the latest GitRepository object
			err = opts.client.Get(ctx, gitRepoNamespacedName, &gitRepo)
			require.NoError(t, err)

			// Update the Status subresource
			updatedStatus := sourcev1.GitRepositoryStatus{
				ObservedGeneration: gitRepo.Generation,
				Artifact: &meta.Artifact{
					URL:      opts.testGitRepoURL,
					Digest:   opts.testGitRepoSHA,
					Revision: fmt.Sprintf("v%d", stepNumber),
					LastUpdateTime: metav1.Time{
						Time: time.Now(),
					},
				},
				Conditions: []metav1.Condition{
					{
						Type:               "Ready",
						Status:             metav1.ConditionTrue,
						ObservedGeneration: gitRepo.Generation,
						Reason:             "Succeeded",
						Message:            "Repository is ready",
						LastTransitionTime: metav1.Time{
							Time: time.Now(),
						},
					},
				},
			}
			gitRepo.Status = updatedStatus
			err = opts.client.Status().Update(ctx, &gitRepo)
			require.NoError(t, err)

			// Now, the FluxController should reconcile the GitRepository and create the DeploymentTemplate resource.
			deploymentTemplateName := name
			deploymentTemplateNamespacedName := types.NamespacedName{Name: deploymentTemplateName, Namespace: namespaceName}
			deploymentTemplate := radappiov1alpha3.DeploymentTemplate{}
			err = waitForDeploymentTemplateToExistWithGeneration(ctx, opts.client, deploymentTemplateNamespacedName, &deploymentTemplate, int64(stepNumber))
			require.NoError(t, err)

			// Fetch the latest DeploymentTemplate object
			err = opts.client.Get(ctx, deploymentTemplateNamespacedName, &deploymentTemplate)
			require.NoError(t, err)
		}
	}
}

func makeGitRepository(namespacedName types.NamespacedName, url string) sourcev1.GitRepository {
	return sourcev1.GitRepository{
		Kind:       "GitRepository",
		APIVersion: "source.toolkit.fluxcd.io/v1",
		Name:       namespacedName.Name,
		Namespace:  namespacedName.Namespace,
		Spec: sourcev1.GitRepositorySpec{
			URL:      url,
			Interval: metav1.Duration{Duration: time.Minute},
		},
	}
}

func waitForGitRepositoryToExistWithGeneration(ctx context.Context, k8sClient k8sclient.Client, key k8sclient.ObjectKey, obj k8sclient.Object, generation int64) error {
	timeout := 10 * time.Second
	interval := 200 * time.Millisecond
	deadlineCtx, deadlineCancel := context.WithTimeout(ctx, timeout)
	defer deadlineCancel()

	err := wait.PollUntilContextTimeout(deadlineCtx, interval, timeout, true, func(_ context.Context) (bool, error) {
		err := k8sClient.Get(ctx, key, obj)
		if err != nil {
			return false, nil // Continue polling
		}

		gitRepo, ok := obj.(*sourcev1.GitRepository)
		if !ok {
			return false, nil // Continue polling
		}

		if gitRepo.Generation != generation {
			return false, nil // Continue polling
		}

		return true, nil // Found
	})

	if err != nil {
		return fmt.Errorf("GitRepository %s/%s was not created successfully", key.Namespace, key.Name)
	}

	return nil
}

func waitForDeploymentTemplateToExistWithGeneration(ctx context.Context, k8sClient k8sclient.Client, key types.NamespacedName, obj k8sclient.Object, generation int64) error {
	timeout := 10 * time.Second
	interval := 200 * time.Millisecond
	deadlineCtx, deadlineCancel := context.WithTimeout(ctx, timeout)
	defer deadlineCancel()

	err := wait.PollUntilContextTimeout(deadlineCtx, interval, timeout, true, func(_ context.Context) (bool, error) {
		err := k8sClient.Get(ctx, key, obj)
		if err != nil {
			return false, nil // Continue polling
		}

		deploymentTemplate, ok := obj.(*radappiov1alpha3.DeploymentTemplate)
		if !ok {
			return false, nil // Continue polling
		}

		if deploymentTemplate.Generation != generation {
			return false, nil // Continue polling
		}

		return true, nil // Found
	})

	if err != nil {
		return fmt.Errorf("DeploymentTemplate %s/%s was not created successfully", key.Namespace, key.Name)
	}

	return nil
}

func Test_isSpecifiedInConfig(t *testing.T) {
	tests := []struct {
		name      string
		fileName  string
		namespace string
		config    []ConfigEntry
		expected  bool
	}{
		{
			name:     "File exists in config",
			fileName: "example.bicep",
			config: []ConfigEntry{
				{Name: "example.bicep"},
				{Name: "another.bicep"},
			},
			expected: true,
		},
		{
			name:     "File does not exist in config",
			fileName: "missing.bicep",
			config: []ConfigEntry{
				{Name: "example.bicep"},
				{Name: "another.bicep"},
			},
			expected: false,
		},
		{
			name:     "Empty config",
			fileName: "example.bicep",
			config:   []ConfigEntry{},
			expected: false,
		},
		{
			name:     "Nil config",
			fileName: "example.bicep",
			config:   nil,
			expected: false,
		},
		{
			name:      "Same filename in another namespace",
			fileName:  "example.bicep",
			namespace: "old-target",
			config:    []ConfigEntry{{Name: "example.bicep", Namespace: "new-target"}},
			expected:  false,
		},
		{
			name:      "Same namespace and filename",
			fileName:  "example.bicep",
			namespace: "target",
			config:    []ConfigEntry{{Name: "example.bicep", Namespace: "target"}},
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSpecifiedInConfig(k8sclient.ObjectKey{Name: tt.fileName, Namespace: tt.namespace}, tt.config)
			require.Equal(t, tt.expected, result)
		})
	}
}

func Test_FluxController_TemplateOwnership(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		change func(*radappiov1alpha3.DeploymentTemplateSourceReference)
		legacy bool
	}{
		{name: "same source"},
		{name: "different source", change: func(ref *radappiov1alpha3.DeploymentTemplateSourceReference) { ref.Name = "other" }},
		{name: "same name different namespace", change: func(ref *radappiov1alpha3.DeploymentTemplateSourceReference) { ref.Namespace = "other" }},
		{name: "recreated source", change: func(ref *radappiov1alpha3.DeploymentTemplateSourceReference) { ref.UID = "old-uid" }},
		{name: "legacy", legacy: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			source := readyFluxSource()
			ref := sourceReference(source)
			owner := ref.DeepCopy()
			if tt.change != nil {
				tt.change(owner)
			}
			if tt.legacy {
				owner = nil
			}
			template := &radappiov1alpha3.DeploymentTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "application.bicep", Namespace: "target", UID: "template-uid"},
				Spec:       radappiov1alpha3.DeploymentTemplateSpec{Template: "before", Repository: source.Name, SourceRef: owner},
			}
			s := newFluxPolicyTestScheme(t)
			c := fake.NewClientBuilder().WithScheme(s).WithObjects(source, template).Build()
			policy, err := NewFluxSourcePolicy(hostoptions.FluxOptions{Enabled: true, Sources: []hostoptions.FluxSourceSelection{{
				Namespace: source.Namespace, Name: source.Name, UID: string(source.UID),
			}}}, c)
			require.NoError(t, err)
			r := &FluxController{Client: c, SourcePolicy: policy}
			before := template.Spec.DeepCopy()
			err = r.createOrUpdateDeploymentTemplate(t.Context(), template.Name, template.Namespace, "after", "config", nil, ref)
			var got radappiov1alpha3.DeploymentTemplate
			require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(template), &got))
			require.Equal(t, template.UID, got.UID)
			if tt.change != nil || tt.legacy {
				require.ErrorContains(t, err, "TemplateSourceMismatch")
				require.Equal(t, *before, got.Spec)
			} else {
				require.NoError(t, err)
				require.Equal(t, "after", got.Spec.Template)
				require.Equal(t, ref, got.Spec.SourceRef)
			}
		})
	}
}
