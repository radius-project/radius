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
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armdeployments/v3"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/pkg/hashutil"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	crconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

const deploymentTemplateTestControllerDelayInterval = time.Millisecond * 100

func SetupDeploymentTemplateTest(t *testing.T) (*mockRadiusClient, *sdkclients.MockResourceDeploymentsClient, k8sclient.Client) {
	t.Helper()
	SkipWithoutEnvironment(t)
	ctx, cancel := context.WithCancel(t.Context())
	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme: scheme,
		Controller: crconfig.Controller{
			SkipNameValidation: new(true),
		},
		Metrics: server.Options{BindAddress: "0"},
	})
	require.NoError(t, err)
	radius := NewMockRadiusClient()
	deployments := sdkclients.NewMockResourceDeploymentsClient()
	//nolint:staticcheck // SA1019: migration to the new events API requires significant refactoring.
	err = (&DeploymentTemplateReconciler{
		APIReader: mgr.GetAPIReader(), Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor("deploymenttemplate-controller"),
		Radius:        radius, ResourceDeploymentsClient: deployments,
		DelayInterval: deploymentTemplateTestControllerDelayInterval,
	}).SetupWithManager(mgr)
	require.NoError(t, err)
	//nolint:staticcheck // SA1019: migration to the new events API requires significant refactoring.
	err = (&DeploymentResourceReconciler{
		APIReader: mgr.GetAPIReader(), Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor("deploymentresource-controller"),
	}).SetupWithManager(mgr)
	require.NoError(t, err)
	startManager(t, mgr, ctx, cancel)
	return radius, deployments, mgr.GetClient()
}

func Test_DeploymentTemplateReconciler_ComputeHash(t *testing.T) {
	for _, tc := range []struct {
		name string
		spec radappiov1alpha3.DeploymentTemplateSpec
		hash string
	}{
		{name: "empty", hash: "44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"},
		{
			name: "simple",
			spec: radappiov1alpha3.DeploymentTemplateSpec{Template: "{}", Parameters: map[string]string{}, ProviderConfig: "{}"},
			hash: "950829dab3e33b280e2ef52a7484c51d19a51498a74aab1d7f4d2781d6e65953",
		},
		{
			name: "complex",
			spec: radappiov1alpha3.DeploymentTemplateSpec{
				Template:       `{"resources":[{"type":"Microsoft.Resources/deployments","apiVersion":"2020-06-01","name":"test-deploymenttemplate-basic","properties":{"mode":"Incremental","template":{},"parameters":{}}}]}`,
				Parameters:     map[string]string{"param1": "value1", "param2": "value2"},
				ProviderConfig: `{"AWS":{"type":"aws","value":{"scope":"scope"}}}`,
			},
			hash: "ff16a7bcb2db8d7c1280eb761b376939dfc3856a74cfd77b986a1c0f25abf5dd",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owner := &radappiov1alpha3.DeploymentTemplate{Spec: tc.spec}
			hash, err := computeHash(owner)
			require.NoError(t, err)
			require.Equal(t, tc.hash, hash)
			owner.Status.StatusHash = hash
			require.True(t, isUpToDate(owner))
			owner.Status.StatusHash = "incorrecthash"
			require.False(t, isUpToDate(owner))
		})
	}
}

func Test_DeploymentTemplateReconciler_IsUpToDate(t *testing.T) {
	owner := &radappiov1alpha3.DeploymentTemplate{
		Spec: radappiov1alpha3.DeploymentTemplateSpec{Template: "{}", Parameters: map[string]string{}, ProviderConfig: "{}"},
		Status: radappiov1alpha3.DeploymentTemplateStatus{
			StatusHash: "47ee899e74561942ee36a02ffd80be955e251583",
		},
	}
	require.True(t, isUpToDate(owner), "legacy SHA-1 hashes must not cause an unnecessary PUT after upgrade")
	hash, err := computeHash(owner)
	require.NoError(t, err)
	require.Equal(t, hash, owner.Status.StatusHash, "a legacy hash is replaced with the current hash, which Reconcile saves")
}

func Test_ParseDeploymentScopeFromProviderConfig(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		config any
		scope  string
	}{
		{name: "string", config: `{"deployments":{"type":"deployments","value":{"scope":"deploymentsscope"}}}`, scope: "deploymentsscope"},
		{name: "bytes", config: []byte(`{"deployments":{"value":{"scope":"deploymentsscope"}}}`), scope: "deploymentsscope"},
		{name: "missing deployments", config: `{"radius":{"value":{"scope":"deploymentsscope"}}}`},
		{name: "empty scope", config: `{"deployments":{"value":{"scope":""}}}`},
		{name: "invalid JSON", config: "{"},
		{name: "invalid type", config: 42},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			scope, err := ParseDeploymentScopeFromProviderConfig(tc.config)
			if tc.scope == "" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.scope, scope)
		})
	}
}

type templateReconcileFixture struct {
	t           *testing.T
	client      k8sclient.WithWatch
	reader      k8sclient.Reader
	scheme      *runtime.Scheme
	deployments *sdkclients.MockResourceDeploymentsClient
	key         types.NamespacedName
}

func newTemplateReconcileFixture(t *testing.T) *templateReconcileFixture {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, radappiov1alpha3.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))
	require.NoError(t, rbacv1.AddToScheme(s))
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{corev1.SchemeGroupVersion, rbacv1.SchemeGroupVersion})
	mapper.Add(corev1.SchemeGroupVersion.WithKind("ConfigMap"), meta.RESTScopeNamespace)
	mapper.Add(corev1.SchemeGroupVersion.WithKind("Namespace"), meta.RESTScopeRoot)
	mapper.Add(rbacv1.SchemeGroupVersion.WithKind("ClusterRole"), meta.RESTScopeRoot)
	key := types.NamespacedName{Namespace: "test", Name: "inventory"}
	providerConfig, err := sdkclients.NewDefaultProviderConfig(key.Namespace).String()
	require.NoError(t, err)
	owner := makeDeploymentTemplate(key, "{}", providerConfig, nil)
	owner.UID = "owner"
	owner.Generation = 1
	c := fake.NewClientBuilder().WithScheme(s).WithRESTMapper(mapper).
		WithStatusSubresource(&radappiov1alpha3.DeploymentTemplate{}, &radappiov1alpha3.DeploymentResource{}).
		WithObjects(owner, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: key.Namespace}}).Build()
	return &templateReconcileFixture{
		t: t, client: c, reader: c, scheme: s,
		deployments: sdkclients.NewMockResourceDeploymentsClient(), key: key,
	}
}

func (f *templateReconcileFixture) owner() *radappiov1alpha3.DeploymentTemplate {
	f.t.Helper()
	owner := &radappiov1alpha3.DeploymentTemplate{}
	require.NoError(f.t, f.client.Get(f.t.Context(), f.key, owner))
	return owner
}

func (f *templateReconcileFixture) step() ctrl.Result {
	f.t.Helper()
	owner := f.owner()
	if state := owner.Status.Coordinator; state != nil && state.Phase == coordinatorDeleteRunning && state.RetryAfter != nil {
		// Advance Kubernetes polling without bypassing terminal-failure backoff.
		past := metav1.NewTime(time.Now().Add(-time.Minute))
		state.RetryAfter = &past
		require.NoError(f.t, f.client.Status().Update(f.t.Context(), owner))
	}
	// Recreate the reconciler so only persisted lifecycle state can drive progress.
	r := &DeploymentTemplateReconciler{
		Client: f.client, APIReader: f.reader, Scheme: f.scheme,
		Radius: NewMockRadiusClient(), ResourceDeploymentsClient: f.deployments,
		EventRecorder: record.NewFakeRecorder(100),
	}
	result, err := r.Reconcile(f.t.Context(), ctrl.Request{NamespacedName: f.key})
	require.NoError(f.t, err)
	return result
}

func (f *templateReconcileFixture) until(predicate func(*radappiov1alpha3.DeploymentTemplate) bool) *radappiov1alpha3.DeploymentTemplate {
	f.t.Helper()
	for range 30 {
		if owner := f.owner(); predicate(owner) {
			return owner
		}
		f.step()
	}
	f.t.Fatalf("template did not reach expected state: status=%+v coordinator=%+v", f.owner().Status, f.owner().Status.Coordinator)
	return nil
}

func (f *templateReconcileFixture) runningPut() *radappiov1alpha3.DeploymentTemplate {
	f.t.Helper()
	return f.until(func(owner *radappiov1alpha3.DeploymentTemplate) bool {
		return owner.Status.Coordinator != nil && owner.Status.Coordinator.Phase == coordinatorPutRunning && owner.Status.Operation != nil
	})
}

func (f *templateReconcileFixture) ready() *radappiov1alpha3.DeploymentTemplate {
	f.t.Helper()
	return f.until(func(owner *radappiov1alpha3.DeploymentTemplate) bool {
		return owner.Status.Phrase == radappiov1alpha3.DeploymentTemplatePhraseReady && owner.Status.Operation == nil
	})
}

func (f *templateReconcileFixture) completePut(ids ...string) {
	f.t.Helper()
	owner := f.owner()
	require.NotNil(f.t, owner.Status.Operation)
	require.EqualValues(f.t, radappiov1alpha3.OperationKindPut, owner.Status.Operation.OperationKind)
	f.deployments.CompleteOperation(owner.Status.Operation.ResumeToken, func(state *sdkclients.OperationState) {
		var refs []*armdeployments.ResourceReference
		for _, id := range ids {
			refs = append(refs, &armdeployments.ResourceReference{ID: new(id)})
		}
		state.Value = sdkclients.ClientCreateOrUpdateResponse{Properties: &armdeployments.DeploymentPropertiesExtended{OutputResources: refs}}
	})
	f.step()
}

func (f *templateReconcileFixture) changeTemplate(template string) {
	f.t.Helper()
	owner := f.owner()
	owner.Spec.Template = template
	owner.Generation++
	require.NoError(f.t, f.client.Update(f.t.Context(), owner))
}

func (f *templateReconcileFixture) expireRetry() {
	f.t.Helper()
	owner := f.owner()
	require.Equal(f.t, coordinatorRetryWaiting, owner.Status.Coordinator.Phase)
	require.NotNil(f.t, owner.Status.Coordinator.RetryAfter)
	require.Positive(f.t, f.step().RequeueAfter)
	require.Equal(f.t, coordinatorRetryWaiting, f.owner().Status.Coordinator.Phase, "retry deadline must survive immediate events")
	owner = f.owner()
	past := metav1.NewTime(time.Now().Add(-time.Minute))
	owner.Status.Coordinator.RetryAfter = &past
	require.NoError(f.t, f.client.Status().Update(f.t.Context(), owner))
}

func (f *templateReconcileFixture) tracker(id string) *radappiov1alpha3.DeploymentResource {
	f.t.Helper()
	children := &radappiov1alpha3.DeploymentResourceList{}
	require.NoError(f.t, f.client.List(f.t.Context(), children, k8sclient.InNamespace(f.key.Namespace)))
	for i := range children.Items {
		if children.Items[i].Spec.Id == id && isOwnedBy(children.Items[i], f.owner()) {
			return &children.Items[i]
		}
	}
	f.t.Fatalf("no owned tracker for %s", id)
	return nil
}

func (f *templateReconcileFixture) drainDeletion() {
	f.t.Helper()
	for range 60 {
		owner := &radappiov1alpha3.DeploymentTemplate{}
		err := f.client.Get(f.t.Context(), f.key, owner)
		if apierrors.IsNotFound(err) {
			return
		}
		require.NoError(f.t, err)
		if owner.Status.Operation != nil {
			require.EqualValues(f.t, radappiov1alpha3.OperationKindDelete, owner.Status.Operation.OperationKind)
			f.deployments.CompleteOperation(owner.Status.Operation.ResumeToken, nil)
		}
		f.step()
	}
	f.t.Fatal("template finalizer did not drain")
}

func Test_DeploymentTemplateReconciler_Basic(t *testing.T) {
	t.Parallel()
	f := newTemplateReconcileFixture(t)
	f.runningPut()
	require.Contains(t, f.owner().Finalizers, DeploymentTemplateFinalizer)
	f.completePut()
	owner := f.ready()
	require.True(t, isUpToDate(owner))
	require.Empty(t, owner.Status.Coordinator.Outputs)
	require.NoError(t, f.client.Delete(t.Context(), owner))
	f.drainDeletion()
}

func Test_DeploymentTemplateReconciler_FailureRecovery(t *testing.T) {
	t.Parallel()
	f := newTemplateReconcileFixture(t)
	first := f.runningPut().Status.Operation.ResumeToken
	f.deployments.CompleteOperation(first, func(state *sdkclients.OperationState) {
		state.Err = errors.New("known terminal PUT failure")
		response := state.Value.(sdkclients.ClientCreateOrUpdateResponse)
		response.Properties.OutputResources = []*armdeployments.ResourceReference{}
		state.Value = response
	})
	f.step()
	require.Equal(t, coordinatorPutInventoryPending, f.owner().Status.Coordinator.Phase)
	result := f.step()
	require.Equal(t, DeleteRetryDelay, result.RequeueAfter)
	owner := f.owner()
	require.Equal(t, coordinatorRetryWaiting, owner.Status.Coordinator.Phase)
	require.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseFailed, owner.Status.Phrase)
	require.Nil(t, owner.Status.Operation)
	require.False(t, isUpToDate(owner))
	f.expireRetry()
	second := f.runningPut().Status.Operation.ResumeToken
	require.NotEqual(t, first, second)
	f.completePut()
	require.True(t, isUpToDate(f.ready()))
}

func Test_DeploymentTemplateReconciler_SpecChangedDuringOperation(t *testing.T) {
	t.Parallel()
	f := newTemplateReconcileFixture(t)
	first := f.runningPut()
	submittedHash := first.Status.Coordinator.SubmittedHash
	f.changeTemplate(`{"resources":[]}`)
	f.step()
	require.Equal(t, first.Status.Operation, f.owner().Status.Operation, "a changed spec must not overlap the running PUT")
	f.completePut()
	require.Equal(t, submittedHash, f.owner().Status.StatusHash)
	second := f.runningPut()
	require.NotEqual(t, first.Status.Operation.ResumeToken, second.Status.Operation.ResumeToken)
	require.NotEqual(t, submittedHash, second.Status.Coordinator.SubmittedHash)
	f.completePut()
	require.True(t, isUpToDate(f.ready()))
}

// A status hash that an older version of Radius computed with SHA-1 is replaced with the current hash and saved without
// a deployment, so the controller no longer needs SHA-1 for the template.
func Test_DeploymentTemplateReconciler_MigratesLegacyStatusHash(t *testing.T) {
	t.Parallel()
	f := newTemplateReconcileFixture(t)
	f.runningPut()
	f.completePut()
	owner := f.ready()
	current := owner.Status.StatusHash
	require.NotEmpty(t, current)

	// Record the status an older version of Radius wrote: the SHA-1 hash of the same spec.
	data, err := json.Marshal(owner.Spec)
	require.NoError(t, err)
	owner.Status.StatusHash = hashutil.LegacyHex(data)
	require.NoError(t, f.client.Status().Update(t.Context(), owner))

	f.step()
	owner = f.owner()
	require.Nil(t, owner.Status.Operation, "a legacy hash of the same spec must not start a deployment")
	require.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseReady, owner.Status.Phrase)
	require.Equal(t, current, owner.Status.StatusHash, "the current hash must be saved")
}

func Test_DeploymentTemplateReconciler_OutputResources(t *testing.T) {
	t.Parallel()
	for _, keepContainer := range []bool{false, true} {
		name := "nil outputs"
		if keepContainer {
			name = "retired dependency"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newTemplateReconcileFixture(t)
			envID := "/planes/radius/local/resourceGroups/test/providers/Applications.Core/environments/environment"
			appID := "/planes/radius/local/resourceGroups/test/providers/Applications.Core/applications/application"
			containerID := "/planes/radius/local/resourceGroups/test/providers/Applications.Core/containers/container"
			f.runningPut()
			f.completePut(envID, appID, containerID, containerID)
			owner := f.owner()
			require.Len(t, owner.Status.Coordinator.Outputs, 3, "duplicate outputs must be deduplicated durably")
			children := &radappiov1alpha3.DeploymentResourceList{}
			require.NoError(t, f.client.List(t.Context(), children))
			require.Empty(t, children.Items, "inventory must be committed before tracker projection")
			f.ready()
			for _, id := range []string{envID, appID, containerID} {
				child := f.tracker(id)
				expectedName, err := generateDeploymentResourceName(id)
				require.NoError(t, err)
				require.Equal(t, expectedName, child.Name)
				require.Equal(t, id, child.Status.ProvisionedResourceID)
				require.Contains(t, child.Finalizers, DeploymentResourceFinalizer)
			}
			f.changeTemplate(`{"resources":[]}`)
			f.runningPut()
			if keepContainer {
				f.completePut(containerID)
				for range 5 {
					f.step()
				}
				owner = f.owner()
				require.Equal(t, outputPendingDelete, findOutput(owner, envID).State)
				require.Nil(t, owner.Status.Operation)
				require.Empty(t, f.deployments.DeletedResourceIDs(), "dependencies must outlive current leaf outputs")
				require.NoError(t, f.client.Delete(t.Context(), owner))
				f.drainDeletion()
			} else {
				f.completePut()
				for range 20 {
					owner = f.owner()
					if owner.Status.Operation != nil {
						require.NotEqual(t, containerID, owner.Status.Coordinator.ResourceID, "leaf DELETE must complete before dependencies")
						f.deployments.CompleteOperation(owner.Status.Operation.ResumeToken, nil)
					}
					if owner.Status.Phrase == radappiov1alpha3.DeploymentTemplatePhraseReady {
						break
					}
					// The first pending DELETE is the leaf, even though dependencies are listed first.
					if len(f.deployments.DeletedResourceIDs()) == 0 {
						f.step()
						owner = f.owner()
						require.Equal(t, containerID, owner.Status.Coordinator.ResourceID)
						f.deployments.CompleteOperation(owner.Status.Operation.ResumeToken, nil)
					}
					f.step()
				}
				require.Empty(t, f.ready().Status.Coordinator.Outputs)
			}
			deleted := f.deployments.DeletedResourceIDs()
			require.Len(t, deleted, 3)
			require.ElementsMatch(t, []string{containerID, envID, appID}, deleted)
		})
	}
}

func Test_DeploymentTemplateReconciler_KubernetesOutputCleanup(t *testing.T) {
	t.Parallel()
	for _, removeOutput := range []bool{false, true} {
		name := "delete-template"
		if removeOutput {
			name = "remove-output"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newTemplateReconcileFixture(t)
			output := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
				Name: "output", Namespace: f.key.Namespace, Finalizers: []string{"example.com/hold"},
			}}
			require.NoError(t, f.client.Create(t.Context(), output))
			id := "/planes/kubernetes/local/namespaces/test/providers/core/ConfigMap/output"
			f.runningPut()
			f.completePut(id)
			f.ready()
			child := f.tracker(id)
			if removeOutput {
				f.changeTemplate(`{"resources":[]}`)
				f.runningPut()
				f.completePut()
			} else {
				require.NoError(t, f.client.Delete(t.Context(), f.owner()))
			}
			owner := f.until(func(owner *radappiov1alpha3.DeploymentTemplate) bool {
				return owner.Status.Coordinator.Phase == coordinatorDeleteRunning
			})
			require.Contains(t, owner.Finalizers, DeploymentTemplateFinalizer)
			require.Equal(t, id, owner.Status.Coordinator.ResourceID)
			require.Nil(t, owner.Status.Operation, "Kubernetes deletion must not create a UCP token")
			require.NoError(t, f.client.Get(t.Context(), k8sclient.ObjectKeyFromObject(output), output))
			require.NotNil(t, output.DeletionTimestamp)
			for range 3 {
				f.step()
				require.NotNil(t, findOutput(f.owner(), id), "external finalizers must retain authoritative inventory")
			}
			require.NoError(t, f.client.Get(t.Context(), k8sclient.ObjectKeyFromObject(output), output))
			output.Finalizers = nil
			require.NoError(t, f.client.Update(t.Context(), output))
			if removeOutput {
				require.Empty(t, f.ready().Status.Coordinator.Outputs)
			} else {
				f.drainDeletion()
			}
			require.True(t, apierrors.IsNotFound(f.client.Get(t.Context(), k8sclient.ObjectKeyFromObject(child), child)))
			require.Empty(t, f.deployments.DeletedResourceIDs())
		})
	}
}

func Test_DeploymentTemplateReconciler_RemoveAndReaddOutput(t *testing.T) {
	t.Parallel()
	f := newTemplateReconcileFixture(t)
	id := "/planes/kubernetes/local/namespaces/test/providers/core/ConfigMap/output"
	output := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name: "output", Namespace: f.key.Namespace, Finalizers: []string{"example.com/hold"},
	}}
	require.NoError(t, f.client.Create(t.Context(), output))
	f.runningPut()
	f.completePut(id)
	f.ready()
	f.changeTemplate(`{"resources":[]}`)
	f.runningPut()
	f.completePut()
	f.until(func(owner *radappiov1alpha3.DeploymentTemplate) bool {
		return owner.Status.Coordinator.Phase == coordinatorDeleteRunning
	})
	f.changeTemplate("{}")
	for range 3 {
		f.step()
		owner := f.owner()
		require.Equal(t, coordinatorDeleteRunning, owner.Status.Coordinator.Phase)
		require.Nil(t, owner.Status.Operation)
		require.NotEqual(t, radappiov1alpha3.DeploymentTemplatePhraseReady, owner.Status.Phrase)
	}
	require.NoError(t, f.client.Get(t.Context(), k8sclient.ObjectKeyFromObject(output), output))
	output.Finalizers = nil
	require.NoError(t, f.client.Update(t.Context(), output))
	f.runningPut()
	recreated := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "output", Namespace: f.key.Namespace}}
	require.NoError(t, f.client.Create(t.Context(), recreated))
	f.completePut(id)
	f.ready()
	for range 3 {
		f.step()
	}
	require.NoError(t, f.client.Get(t.Context(), k8sclient.ObjectKeyFromObject(recreated), recreated))
	require.Nil(t, recreated.DeletionTimestamp, "old cleanup must not delete a reintroduced output")
	require.Equal(t, id, f.tracker(id).Status.ProvisionedResourceID)
}

func Test_DeploymentTemplateReconciler_CompletedPutWithTerminatingTracker(t *testing.T) {
	t.Parallel()
	for _, deleteDuringPut := range []bool{false, true} {
		t.Run(map[bool]string{false: "repair output", true: "delete template"}[deleteDuringPut], func(t *testing.T) {
			t.Parallel()
			f := newTemplateReconcileFixture(t)
			firstID := "/planes/kubernetes/local/namespaces/test/providers/core/ConfigMap/first"
			secondID := "/planes/kubernetes/local/namespaces/test/providers/core/ConfigMap/second"
			for _, name := range []string{"first", "second"} {
				require.NoError(t, f.client.Create(t.Context(), &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: f.key.Namespace}}))
			}
			f.runningPut()
			f.completePut(firstID)
			f.ready()
			f.changeTemplate(`{"resources":[]}`)
			f.runningPut()
			child := f.tracker(firstID)
			require.NoError(t, f.client.Delete(t.Context(), child))
			if deleteDuringPut {
				require.NoError(t, f.client.Delete(t.Context(), f.owner()))
			}
			f.completePut(firstID, secondID)
			require.ElementsMatch(t, []string{firstID, secondID}, f.owner().Status.OutputResources)
			require.Nil(t, f.owner().Status.Operation, "completed inventory must not depend on tracker retirement")
			if deleteDuringPut {
				f.drainDeletion()
				require.True(t, apierrors.IsNotFound(f.client.Get(t.Context(), types.NamespacedName{Namespace: "test", Name: "second"}, &corev1.ConfigMap{})))
			} else {
				f.runningPut()
				require.Empty(t, f.owner().Status.StatusHash, "deleting a current output must force a repair PUT")
				f.completePut(firstID, secondID)
				require.True(t, isUpToDate(f.ready()))
			}
		})
	}
}

func Test_DeploymentTemplateReconciler_ExistingTrackerAtQuota(t *testing.T) {
	t.Parallel()
	f := newTemplateReconcileFixture(t)
	id := "/planes/kubernetes/local/namespaces/test/providers/core/ConfigMap/output"
	f.runningPut()
	child := makeDeploymentResource(types.NamespacedName{Namespace: "test", Name: "legacy-output"}, id)
	child.Finalizers = []string{DeploymentResourceFinalizer}
	require.NoError(t, controllerutil.SetControllerReference(f.owner(), child, f.scheme))
	require.NoError(t, f.client.Create(t.Context(), child))
	creates := 0
	f.client = interceptor.NewClient(f.client, interceptor.Funcs{
		Create: func(ctx context.Context, c k8sclient.WithWatch, obj k8sclient.Object, opts ...k8sclient.CreateOption) error {
			if _, ok := obj.(*radappiov1alpha3.DeploymentResource); ok {
				creates++
				return apierrors.NewForbidden(schema.GroupResource{Group: "radapp.io", Resource: "deploymentresources"}, obj.GetName(), errors.New("quota exceeded"))
			}
			return c.Create(ctx, obj, opts...)
		},
	})
	f.completePut(id)
	f.ready()
	require.Zero(t, creates, "legacy tracker identity must be adopted without duplicate POST at quota")
	require.Equal(t, child.Name, f.tracker(id).Name)
	require.Equal(t, id, f.tracker(id).Status.ProvisionedResourceID)
}

func Test_DeploymentTemplateReconciler_RejectsNonControllerTracker(t *testing.T) {
	t.Parallel()
	f := newTemplateReconcileFixture(t)
	id := "/planes/kubernetes/local/namespaces/test/providers/core/ConfigMap/output"
	name, err := generateDeploymentResourceName(id)
	require.NoError(t, err)
	child := makeDeploymentResource(types.NamespacedName{Namespace: "test", Name: name}, id)
	require.NoError(t, controllerutil.SetControllerReference(f.owner(), child, f.scheme))
	child.OwnerReferences[0].Controller = new(false)
	child.Finalizers = []string{DeploymentResourceFinalizer}
	require.NoError(t, f.client.Create(t.Context(), child))
	f.runningPut()
	f.completePut(id)
	r := &DeploymentTemplateReconciler{Client: f.client, APIReader: f.reader, Scheme: f.scheme}
	_, err = r.Reconcile(t.Context(), ctrl.Request{NamespacedName: f.key})
	require.ErrorContains(t, err, "does not match")
	require.NoError(t, f.client.Get(t.Context(), k8sclient.ObjectKeyFromObject(child), child))
	require.Empty(t, child.Status.ProvisionedResourceID)
	require.Nil(t, child.DeletionTimestamp)
	require.NotNil(t, findOutput(f.owner(), id), "a projection conflict must not discard completed inventory")
}

func Test_DeploymentTemplateReconciler_DeleteInventoryBypassesCache(t *testing.T) {
	t.Parallel()
	f := newTemplateReconcileFixture(t)
	id := "/planes/kubernetes/local/namespaces/test/providers/core/ConfigMap/output"
	f.runningPut()
	f.completePut(id)
	f.ready()
	child := f.tracker(id)
	underlying := f.client
	f.client = interceptor.NewClient(underlying, interceptor.Funcs{
		List: func(ctx context.Context, c k8sclient.WithWatch, list k8sclient.ObjectList, opts ...k8sclient.ListOption) error {
			if children, ok := list.(*radappiov1alpha3.DeploymentResourceList); ok {
				children.Items = nil
				return nil
			}
			return c.List(ctx, list, opts...)
		},
	})
	require.NoError(t, underlying.Delete(t.Context(), f.owner()))
	f.drainDeletion()
	require.True(t, apierrors.IsNotFound(underlying.Get(t.Context(), k8sclient.ObjectKeyFromObject(child), child)), "uncached inventory must retire the real tracker before finalizing")
}

func Test_DeploymentTemplateReconciler_DisappearingCompletedOutputTracker(t *testing.T) {
	t.Parallel()
	f := newTemplateReconcileFixture(t)
	id := "/planes/kubernetes/local/namespaces/test/providers/core/ConfigMap/output"
	f.runningPut()
	f.completePut(id)
	f.ready()
	child := f.tracker(id)
	child.Finalizers = nil
	require.NoError(t, f.client.Update(t.Context(), child))
	require.NoError(t, f.client.Delete(t.Context(), child))
	for range 3 {
		f.step()
	}
	require.Equal(t, id, f.tracker(id).Status.ProvisionedResourceID)
	require.Nil(t, f.owner().Status.Operation, "a missing projection must not redeploy an intact output")
	require.Empty(t, f.deployments.DeletedResourceIDs())
	require.True(t, isUpToDate(f.owner()))
}

func Test_DeploymentTemplateReconciler_NamespaceTerminatesDuringPut(t *testing.T) {
	t.Parallel()
	for _, templateFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "namespace first", true: "template first"}[templateFirst], func(t *testing.T) {
			t.Parallel()
			f := newTemplateReconcileFixture(t)
			oldID := "/planes/kubernetes/local/namespaces/target/providers/core/ConfigMap/old"
			cmID := "/planes/kubernetes/local/namespaces/target/providers/core/ConfigMap/output"
			roleID := "/planes/kubernetes/local/providers/rbac.authorization.k8s.io/ClusterRole/output"
			envID := "/planes/radius/local/resourceGroups/test/providers/Applications.Core/environments/environment"
			redisID := "/planes/radius/local/resourceGroups/test/providers/Applications.Datastores/redisCaches/redis"
			for _, name := range []string{"old", "output"} {
				require.NoError(t, f.client.Create(t.Context(), &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "target"}}))
			}
			require.NoError(t, f.client.Create(t.Context(), &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "output"}}))
			f.runningPut()
			f.completePut(oldID, envID)
			f.ready()
			f.changeTemplate(`{"resources":[]}`)
			f.runningPut()
			ns := &corev1.Namespace{}
			require.NoError(t, f.client.Get(t.Context(), types.NamespacedName{Name: f.key.Namespace}, ns))
			ns.Finalizers = []string{"example.com/hold"}
			require.NoError(t, f.client.Update(t.Context(), ns))
			require.NoError(t, f.client.Delete(t.Context(), ns))
			if templateFirst {
				require.NoError(t, f.client.Delete(t.Context(), f.owner()))
			}
			childCreates := 0
			f.client = interceptor.NewClient(f.client, interceptor.Funcs{
				Create: func(ctx context.Context, c k8sclient.WithWatch, obj k8sclient.Object, opts ...k8sclient.CreateOption) error {
					if _, ok := obj.(*radappiov1alpha3.DeploymentResource); ok {
						childCreates++
						return apierrors.NewForbidden(schema.GroupResource{Resource: "deploymentresources"}, obj.GetName(), errors.New("namespace is terminating"))
					}
					return c.Create(ctx, obj, opts...)
				},
			})
			f.completePut(envID, redisID, cmID, roleID)
			require.Len(t, f.owner().Status.Coordinator.Outputs, 5, "retired and newly completed outputs must survive namespace termination")
			for range 60 {
				owner := &radappiov1alpha3.DeploymentTemplate{}
				err := f.client.Get(t.Context(), f.key, owner)
				if apierrors.IsNotFound(err) {
					break
				}
				require.NoError(t, err)
				if owner.Status.Operation != nil {
					require.EqualValues(t, radappiov1alpha3.OperationKindDelete, owner.Status.Operation.OperationKind)
					if owner.Status.Coordinator.ResourceID == envID {
						require.Nil(t, findOutput(owner, redisID), "tracked environment must outlive untracked Redis")
						require.Nil(t, findOutput(owner, cmID))
						require.Nil(t, findOutput(owner, roleID))
					}
					f.deployments.CompleteOperation(owner.Status.Operation.ResumeToken, nil)
				}
				f.step()
			}
			require.True(t, apierrors.IsNotFound(f.client.Get(t.Context(), f.key, &radappiov1alpha3.DeploymentTemplate{})))
			require.Zero(t, childCreates, "cleanup must not need new trackers in a terminating namespace")
			require.ElementsMatch(t, []string{redisID, envID}, f.deployments.DeletedResourceIDs())
			for _, name := range []string{"old", "output"} {
				require.True(t, apierrors.IsNotFound(f.client.Get(t.Context(), types.NamespacedName{Namespace: "target", Name: name}, &corev1.ConfigMap{})))
			}
			require.True(t, apierrors.IsNotFound(f.client.Get(t.Context(), types.NamespacedName{Name: "output"}, &rbacv1.ClusterRole{})))
		})
	}
}

func Test_DeploymentTemplateReconciler_UntrackedOutputCleanupFailuresAndScope(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		id   string
		fail bool
	}{
		{name: "known terminal Radius failure", id: "/planes/radius/local/resourceGroups/test/providers/Applications.Core/containers/output", fail: true},
		{name: "Radius outside scope", id: "/planes/radius/local/resourceGroups/other/providers/Applications.Core/containers/output"},
		{name: "remote Kubernetes cluster", id: "/planes/kubernetes/remote/namespaces/test/providers/core/ConfigMap/output"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newTemplateReconcileFixture(t)
			f.runningPut()
			f.completePut(tc.id)
			require.NoError(t, f.client.Delete(t.Context(), f.owner()))
			if tc.fail {
				owner := f.until(func(owner *radappiov1alpha3.DeploymentTemplate) bool {
					return owner.Status.Coordinator.Phase == coordinatorDeleteRunning
				})
				first := owner.Status.Operation.ResumeToken
				f.deployments.CompleteOperation(first, func(state *sdkclients.OperationState) {
					state.Err = errors.New("known terminal DELETE failure")
				})
				require.Equal(t, DeleteRetryDelay, f.step().RequeueAfter)
				owner = f.owner()
				require.Equal(t, coordinatorRetryWaiting, owner.Status.Coordinator.Phase)
				require.NotNil(t, findOutput(owner, tc.id))
				require.Nil(t, owner.Status.Operation)
				require.Contains(t, owner.Finalizers, DeploymentTemplateFinalizer)
				f.expireRetry()
				owner = f.until(func(owner *radappiov1alpha3.DeploymentTemplate) bool {
					return owner.Status.Operation != nil
				})
				require.NotEqual(t, first, owner.Status.Operation.ResumeToken)
			}
			f.drainDeletion()
			if !tc.fail {
				require.Empty(t, f.deployments.DeletedResourceIDs(), "out-of-scope inventory must only be forgotten")
			}
		})
	}
}

func Test_DeploymentTemplateReconciler_KubernetesCleanupFailure(t *testing.T) {
	t.Parallel()
	for _, forbidden := range []bool{false, true} {
		t.Run(map[bool]string{false: "uncertain outcome", true: "authorization failure"}[forbidden], func(t *testing.T) {
			t.Parallel()
			f := newTemplateReconcileFixture(t)
			id := "/planes/kubernetes/local/namespaces/test/providers/core/ConfigMap/output"
			output := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "output", Namespace: "test"}}
			require.NoError(t, f.client.Create(t.Context(), output))
			f.runningPut()
			f.completePut(id)
			require.NoError(t, f.client.Delete(t.Context(), f.owner()))
			deletes := 0
			deny := true
			f.client = interceptor.NewClient(f.client, interceptor.Funcs{
				Delete: func(ctx context.Context, c k8sclient.WithWatch, obj k8sclient.Object, opts ...k8sclient.DeleteOption) error {
					if obj.GetObjectKind().GroupVersionKind().Kind == "ConfigMap" {
						deletes++
						if deny {
							if forbidden {
								return apierrors.NewForbidden(schema.GroupResource{Resource: "configmaps"}, obj.GetName(), errors.New("denied"))
							}
							return errors.New("connection lost after sending DELETE")
						}
					}
					return c.Delete(ctx, obj, opts...)
				},
			})
			owner := f.until(func(owner *radappiov1alpha3.DeploymentTemplate) bool {
				return owner.Status.Phrase == radappiov1alpha3.DeploymentTemplatePhraseFailed
			})
			require.NotNil(t, findOutput(owner, id))
			require.Contains(t, owner.Finalizers, DeploymentTemplateFinalizer)
			require.Nil(t, owner.Status.Operation)
			require.NoError(t, f.client.Get(t.Context(), k8sclient.ObjectKeyFromObject(output), output))
			if forbidden {
				require.Equal(t, coordinatorRetryWaiting, owner.Status.Coordinator.Phase)
				f.expireRetry()
				deny = false
				f.drainDeletion()
			} else {
				require.Equal(t, coordinatorBlocked, owner.Status.Coordinator.Phase)
				deny = false
				for range 4 {
					f.step()
				}
				require.Equal(t, 1, deletes, "an ambiguous DELETE must never be replayed automatically")
				require.NotNil(t, findOutput(f.owner(), id))
			}
			require.Empty(t, f.deployments.DeletedResourceIDs())
		})
	}
}

func Test_DeploymentTemplateReconciler_AdoptsLegacyPut(t *testing.T) {
	t.Parallel()
	f := newTemplateReconcileFixture(t)
	first := f.runningPut()
	token := first.Status.Operation.ResumeToken
	first.Status.Coordinator = nil
	require.NoError(t, f.client.Status().Update(t.Context(), first))
	f.step()
	owner := f.owner()
	require.Equal(t, coordinatorPutRunning, owner.Status.Coordinator.Phase)
	require.Equal(t, token, owner.Status.Operation.ResumeToken)
	require.Empty(t, owner.Status.Coordinator.SubmittedHash, "legacy operations have no trustworthy submitted hash")
	f.completePut()
	require.Empty(t, f.owner().Status.StatusHash)
	next := f.runningPut()
	require.NotEqual(t, token, next.Status.Operation.ResumeToken, "unknown legacy spec must be repaired once, not accepted as current")
	f.completePut()
	f.ready()
	for range 4 {
		f.step()
		require.Nil(t, f.owner().Status.Operation, "the repair PUT must not loop")
		require.True(t, isUpToDate(f.owner()))
	}
}

func Test_DeploymentTemplateReconciler_UpdateUnblocksDependencyCleanup(t *testing.T) {
	t.Parallel()
	for _, readdEnvironment := range []bool{false, true} {
		t.Run(map[bool]string{false: "remove leaf", true: "reintroduce dependency"}[readdEnvironment], func(t *testing.T) {
			t.Parallel()
			f := newTemplateReconcileFixture(t)
			envID := "/planes/radius/local/resourceGroups/test/providers/Applications.Core/environments/environment"
			containerID := "/planes/radius/local/resourceGroups/test/providers/Applications.Core/containers/container"
			f.runningPut()
			f.completePut(envID, containerID)
			f.ready()
			f.changeTemplate(`{"resources":[]}`)
			f.runningPut()
			f.completePut(containerID)
			for range 4 {
				f.step()
			}
			owner := f.owner()
			require.Equal(t, outputPendingDelete, findOutput(owner, envID).State)
			require.Nil(t, owner.Status.Operation)
			require.Empty(t, f.deployments.DeletedResourceIDs())
			f.changeTemplate(`{"resources":{}}`)
			f.runningPut()
			if readdEnvironment {
				f.completePut(envID, containerID)
				f.ready()
				for range 5 {
					f.step()
					require.Nil(t, f.owner().Status.Operation, "reintroduced dependency must not start a PUT loop")
					require.True(t, isUpToDate(f.owner()))
					require.Equal(t, outputCurrent, findOutput(f.owner(), envID).State)
				}
				require.Empty(t, f.deployments.DeletedResourceIDs())
				require.NoError(t, f.client.Delete(t.Context(), f.owner()))
				f.drainDeletion()
			} else {
				f.completePut()
				for range 20 {
					owner = f.owner()
					if owner.Status.Operation != nil {
						if owner.Status.Coordinator.ResourceID == envID {
							require.Nil(t, findOutput(owner, containerID), "leaf cleanup must complete before dependency deletion")
						}
						f.deployments.CompleteOperation(owner.Status.Operation.ResumeToken, nil)
					}
					if owner.Status.Phrase == radappiov1alpha3.DeploymentTemplatePhraseReady {
						break
					}
					f.step()
				}
				require.Empty(t, f.ready().Status.Coordinator.Outputs)
			}
			require.ElementsMatch(t, []string{containerID, envID}, f.deployments.DeletedResourceIDs())
		})
	}
}

func Test_DeploymentTemplateReconciler_DependencyCleanupBarrier(t *testing.T) {
	t.Parallel()
	f := newTemplateReconcileFixture(t)
	id := "/planes/radius/local/resourceGroups/test/providers/Applications.Core/containers/container"
	f.runningPut()
	f.completePut(id)
	f.ready()
	f.changeTemplate(`{"resources":[]}`)
	f.runningPut()
	f.completePut()
	first := f.until(func(owner *radappiov1alpha3.DeploymentTemplate) bool {
		return owner.Status.Coordinator.Phase == coordinatorDeleteRunning
	}).Status.Operation
	f.changeTemplate(`{"resources":{}}`)
	for range 4 {
		f.step()
		require.Equal(t, first, f.owner().Status.Operation, "spec updates must never overlap an outstanding DELETE")
		require.Equal(t, coordinatorDeleteRunning, f.owner().Status.Coordinator.Phase)
	}
	f.deployments.CompleteOperation(first.ResumeToken, nil)
	second := f.runningPut().Status.Operation
	require.EqualValues(t, radappiov1alpha3.OperationKindPut, second.OperationKind)
	require.NotEqual(t, first.ResumeToken, second.ResumeToken)
}

func Test_DeploymentTemplateReconciler_SelfContainingNamespaceCleanup(t *testing.T) {
	t.Parallel()
	f := newTemplateReconcileFixture(t)
	ns := &corev1.Namespace{}
	require.NoError(t, f.client.Get(t.Context(), types.NamespacedName{Name: "test"}, ns))
	ns.Finalizers = []string{"example.com/hold"}
	require.NoError(t, f.client.Update(t.Context(), ns))
	f.runningPut()
	id := "/planes/kubernetes/local/providers/core/Namespace/test"
	f.completePut(id)
	f.ready()
	require.NoError(t, f.client.Delete(t.Context(), f.owner()))
	f.drainDeletion()
	require.NoError(t, f.client.Get(t.Context(), types.NamespacedName{Name: "test"}, ns))
	require.NotNil(t, ns.DeletionTimestamp, "template must not wait for its own namespace content deletion")
	require.Empty(t, f.deployments.DeletedResourceIDs())
}

type sourceTestDeployments struct {
	sdkclients.ResourceDeploymentsClient
	creates int
	deletes int
}

func (c *sourceTestDeployments) CreateOrUpdate(ctx context.Context, deployment sdkclients.Deployment, id, version string) (sdkclients.Poller[sdkclients.ClientCreateOrUpdateResponse], error) {
	c.creates++
	return c.ResourceDeploymentsClient.CreateOrUpdate(ctx, deployment, id, version)
}

func (c *sourceTestDeployments) Delete(ctx context.Context, id, version string) (sdkclients.Poller[sdkclients.ClientDeleteResponse], error) {
	c.deletes++
	return c.ResourceDeploymentsClient.Delete(ctx, id, version)
}

type sourceTestRadius struct {
	RadiusClient
	groupCalls int
}

func (c *sourceTestRadius) Groups(scope string) ResourceGroupClient {
	c.groupCalls++
	return c.RadiusClient.Groups(scope)
}

func selectedSourcePolicy(t *testing.T, c k8sclient.Reader, source *sourcev1.GitRepository) *FluxSourcePolicy {
	t.Helper()
	policy, err := NewFluxSourcePolicy(hostoptions.FluxOptions{Enabled: true, Sources: []hostoptions.FluxSourceSelection{{
		Namespace: source.Namespace, Name: source.Name, UID: string(source.UID),
	}}}, c)
	require.NoError(t, err)
	return policy
}

// sourceManagedTemplate returns an adopted, idle template generated from source.
func sourceManagedTemplate(t *testing.T, source *sourcev1.GitRepository) *radappiov1alpha3.DeploymentTemplate {
	t.Helper()
	provider, err := sdkclients.NewDefaultProviderConfig("selected").String()
	require.NoError(t, err)
	return &radappiov1alpha3.DeploymentTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "application", Namespace: "target", UID: "template", Finalizers: []string{DeploymentTemplateFinalizer}},
		Spec: radappiov1alpha3.DeploymentTemplateSpec{
			Template: `{"resources":[]}`, ProviderConfig: provider, Repository: source.Name, SourceRef: sourceReference(source),
		},
		Status: radappiov1alpha3.DeploymentTemplateStatus{
			Phrase:      radappiov1alpha3.DeploymentTemplatePhraseReady,
			Coordinator: &radappiov1alpha3.DeploymentCoordinatorStatus{Phase: coordinatorIdle},
		},
	}
}

func newSourceTestClient(t *testing.T, objects ...k8sclient.Object) *fluxWriteCounter {
	t.Helper()
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{corev1.SchemeGroupVersion})
	mapper.Add(corev1.SchemeGroupVersion.WithKind("ConfigMap"), meta.RESTScopeNamespace)
	mapper.Add(corev1.SchemeGroupVersion.WithKind("Namespace"), meta.RESTScopeRoot)
	objects = append(objects, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "target"}})
	return &fluxWriteCounter{Client: fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).WithRESTMapper(mapper).
		WithStatusSubresource(&radappiov1alpha3.DeploymentTemplate{}, &radappiov1alpha3.DeploymentResource{}).
		WithObjects(objects...).Build()}
}

func Test_DeploymentTemplateReconciler_SourceState(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name      string
		enabled   bool
		unadopted bool
		change    func(*radappiov1alpha3.DeploymentTemplate, *sourcev1.GitRepository)
		reason    string
	}{
		{name: "selected", enabled: true},
		{name: "disabled", reason: "SourceNotSelected"},
		{name: "legacy", change: func(dt *radappiov1alpha3.DeploymentTemplate, _ *sourcev1.GitRepository) { dt.Spec.SourceRef = nil }, reason: "TemplateSourceRequired"},
		{name: "unadopted legacy", unadopted: true, change: func(dt *radappiov1alpha3.DeploymentTemplate, _ *sourcev1.GitRepository) {
			dt.Spec.SourceRef = nil
		}, reason: "TemplateSourceRequired"},
		{name: "stale generation", enabled: true, change: func(dt *radappiov1alpha3.DeploymentTemplate, _ *sourcev1.GitRepository) {
			dt.Spec.SourceRef.Generation++
		}, reason: "SourceIdentityChanged"},
		{name: "suspended", enabled: true, change: func(_ *radappiov1alpha3.DeploymentTemplate, s *sourcev1.GitRepository) { s.Spec.Suspend = true }, reason: "SourceNotReady"},
		{name: "inconsistent", enabled: true, change: func(dt *radappiov1alpha3.DeploymentTemplate, _ *sourcev1.GitRepository) {
			dt.Spec.Repository = "different"
		}, reason: "TemplateSourceRequired"},
		{name: "unsupported kind", enabled: true, change: func(dt *radappiov1alpha3.DeploymentTemplate, _ *sourcev1.GitRepository) {
			dt.Spec.SourceRef.Kind = "OCIRepository"
		}, reason: "TemplateSourceRequired"},
		{name: "omitted kind", enabled: true, change: func(dt *radappiov1alpha3.DeploymentTemplate, _ *sourcev1.GitRepository) {
			dt.Spec.SourceRef.Kind = ""
		}},
		{name: "trusted direct", change: func(dt *radappiov1alpha3.DeploymentTemplate, _ *sourcev1.GitRepository) {
			dt.Spec.Repository = ""
			dt.Spec.SourceRef = nil
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			source := readyFluxSource()
			dt := sourceManagedTemplate(t, source)
			dt.Spec.Parameters = map[string]string{"secret": "never-log-this-value"}
			if tt.unadopted {
				dt.Finalizers, dt.Status = nil, radappiov1alpha3.DeploymentTemplateStatus{}
			}
			if tt.change != nil {
				tt.change(dt, source)
			}
			c := newSourceTestClient(t, source, dt)
			backend := &sourceTestDeployments{ResourceDeploymentsClient: sdkclients.NewMockResourceDeploymentsClient()}
			radius := &sourceTestRadius{RadiusClient: NewMockRadiusClient()}
			events := record.NewFakeRecorder(10)
			retryDelay := 43 * time.Second
			r := &DeploymentTemplateReconciler{Client: c, Scheme: newFluxPolicyTestScheme(t), EventRecorder: events, Radius: radius,
				ResourceDeploymentsClient: backend, DeleteRetryInterval: retryDelay}
			if tt.enabled {
				r.SourcePolicy = selectedSourcePolicy(t, c, source)
			}
			request := ctrl.Request{NamespacedName: k8sclient.ObjectKeyFromObject(dt)}
			result, err := r.Reconcile(t.Context(), request)
			require.NoError(t, err)
			require.Positive(t, result.RequeueAfter)
			var got radappiov1alpha3.DeploymentTemplate
			require.NoError(t, c.Get(t.Context(), request.NamespacedName, &got))
			require.Equal(t, dt.Spec, got.Spec)
			require.Equal(t, dt.Finalizers, got.Finalizers)
			if tt.reason == "" {
				require.Equal(t, 1, backend.creates)
				require.Positive(t, radius.groupCalls)
				require.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseUpdating, got.Status.Phrase)
				require.Equal(t, coordinatorPutRunning, got.Status.Coordinator.Phase)
				require.NotEmpty(t, got.Status.Coordinator.SubmittedHash)
				return
			}
			require.Equal(t, retryDelay, result.RequeueAfter)
			require.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseHeld, got.Status.Phrase)
			require.Equal(t, dt.Status.Coordinator, got.Status.Coordinator, "a hold must not change lifecycle state")
			require.Zero(t, backend.creates)
			require.Zero(t, backend.deletes)
			require.Zero(t, radius.groupCalls)
			require.Zero(t, c.creates)
			require.Zero(t, c.updates, "a held template must not be adopted or changed")
			require.Zero(t, c.deletes)
			select {
			case event := <-events.Events:
				require.Contains(t, event, tt.reason)
				require.NotContains(t, event, "never-log-this-value")
			default:
				t.Fatal("missing hold diagnostic")
			}
			updated := &radappiov1alpha3.DeploymentTemplate{}
			require.NoError(t, c.Get(t.Context(), request.NamespacedName, updated))
			updated.Spec.SourceRef = nil
			require.NoError(t, c.Update(t.Context(), updated))
			result, err = r.Reconcile(t.Context(), request)
			require.NoError(t, err)
			require.Equal(t, retryDelay, result.RequeueAfter)
			select {
			case event := <-events.Events:
				require.Contains(t, event, "TemplateSourceRequired", "a repeated hold must refresh its current reason")
			default:
				t.Fatal("missing updated hold diagnostic")
			}
		})
	}
}

func Test_DeploymentTemplateReconciler_HoldRecordsAcceptedOperation(t *testing.T) {
	t.Parallel()
	source := readyFluxSource()
	dt := sourceManagedTemplate(t, source)
	submittedHash, err := computeHash(dt)
	require.NoError(t, err)
	mock := sdkclients.NewMockResourceDeploymentsClient()
	deploymentID := "/planes/radius/local/resourceGroups/selected/providers/Microsoft.Resources/deployments/accepted"
	poller, err := mock.CreateOrUpdate(t.Context(), sdkclients.Deployment{}, deploymentID, "test")
	require.NoError(t, err)
	token, err := poller.ResumeToken()
	require.NoError(t, err)
	outputID := "/planes/radius/local/resourceGroups/selected/providers/Applications.Core/containers/output"
	mock.CompleteOperation(token, func(state *sdkclients.OperationState) {
		state.Value = sdkclients.ClientCreateOrUpdateResponse{Properties: &armdeployments.DeploymentPropertiesExtended{
			OutputResources: []*armdeployments.ResourceReference{{ID: new(outputID)}},
		}}
	})
	dt.Status.Operation = &radappiov1alpha3.ResourceOperation{OperationKind: radappiov1alpha3.OperationKindPut, ResumeToken: token}
	dt.Status.Coordinator = &radappiov1alpha3.DeploymentCoordinatorStatus{Phase: coordinatorPutRunning, ResourceID: deploymentID, SubmittedHash: submittedHash}
	dt.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseUpdating
	c := newSourceTestClient(t, source, dt)
	backend := &sourceTestDeployments{ResourceDeploymentsClient: mock}
	r := &DeploymentTemplateReconciler{Client: c, Scheme: newFluxPolicyTestScheme(t), EventRecorder: record.NewFakeRecorder(10),
		Radius: NewMockRadiusClient(), ResourceDeploymentsClient: backend}
	request := ctrl.Request{NamespacedName: k8sclient.ObjectKeyFromObject(dt)}
	for range 3 {
		_, err = r.Reconcile(t.Context(), request)
		require.NoError(t, err)
	}
	var held radappiov1alpha3.DeploymentTemplate
	require.NoError(t, c.Get(t.Context(), request.NamespacedName, &held))
	require.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseHeld, held.Status.Phrase)
	require.Nil(t, held.Status.Operation, "a hold must not discard an accepted operation's outcome")
	require.Equal(t, submittedHash, held.Status.StatusHash)
	require.Equal(t, []radappiov1alpha3.DeploymentOutputStatus{{ID: outputID, State: outputCurrent}}, held.Status.Coordinator.Outputs)
	require.Zero(t, c.creates, "a held template must not project trackers")
	require.Zero(t, c.deletes)
	require.Zero(t, backend.creates)
	require.Zero(t, backend.deletes)

	r.SourcePolicy = selectedSourcePolicy(t, c, source)
	for range 3 {
		_, err = r.Reconcile(t.Context(), request)
		require.NoError(t, err)
	}
	var resumed radappiov1alpha3.DeploymentTemplate
	require.NoError(t, c.Get(t.Context(), request.NamespacedName, &resumed))
	require.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseReady, resumed.Status.Phrase)
	require.Equal(t, 1, c.creates, "an active source projects the recorded output")
	require.Zero(t, backend.creates, "an unchanged template must not redeploy")
	name, err := generateDeploymentResourceName(outputID)
	require.NoError(t, err)
	tracker := &radappiov1alpha3.DeploymentResource{}
	require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKey{Namespace: dt.Namespace, Name: name}, tracker))
	require.Equal(t, outputID, tracker.Status.ProvisionedResourceID)
}

func Test_DeploymentTemplateReconciler_HoldDeletion(t *testing.T) {
	t.Parallel()
	source := readyFluxSource()
	outputID := "/planes/radius/local/resourceGroups/selected/providers/Applications.Core/containers/output"
	owner := sourceManagedTemplate(t, source)
	owner.DeletionTimestamp = new(metav1.Now())
	owner.Status.OutputResources = []string{outputID}
	owner.Status.Coordinator.Outputs = []radappiov1alpha3.DeploymentOutputStatus{{ID: outputID, State: outputCurrent}}
	child := &radappiov1alpha3.DeploymentResource{
		ObjectMeta: metav1.ObjectMeta{Name: "output", Namespace: owner.Namespace, UID: "output", Finalizers: []string{DeploymentResourceFinalizer},
			OwnerReferences: []metav1.OwnerReference{{APIVersion: radappiov1alpha3.GroupVersion.String(), Kind: deploymentTemplateKind,
				Name: owner.Name, UID: owner.UID, Controller: new(true)}}},
		Spec:   radappiov1alpha3.DeploymentResourceSpec{Id: outputID},
		Status: radappiov1alpha3.DeploymentResourceStatus{ProvisionedResourceID: outputID},
	}
	c := newSourceTestClient(t, source, owner, child)
	backend := &sourceTestDeployments{ResourceDeploymentsClient: sdkclients.NewMockResourceDeploymentsClient()}
	r := &DeploymentTemplateReconciler{Client: c, EventRecorder: record.NewFakeRecorder(5), ResourceDeploymentsClient: backend}
	_, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: k8sclient.ObjectKeyFromObject(owner)})
	require.NoError(t, err)
	require.Zero(t, c.deletes)
	require.Zero(t, c.updates)
	require.Zero(t, backend.deletes)
	var got radappiov1alpha3.DeploymentTemplate
	require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(owner), &got))
	require.Equal(t, owner.Finalizers, got.Finalizers)
	require.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseHeld, got.Status.Phrase)
	require.Equal(t, owner.Status.Coordinator, got.Status.Coordinator, "held cleanup must not change lifecycle state")
	require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(child), child))
	require.Contains(t, child.Finalizers, DeploymentResourceFinalizer)
}

func Test_DeploymentTemplateReconciler_HoldRunningKubernetesDelete(t *testing.T) {
	t.Parallel()
	source := readyFluxSource()
	outputID := "/planes/kubernetes/local/namespaces/target/providers/core/ConfigMap/output"
	owner := sourceManagedTemplate(t, source)
	owner.Status.Coordinator = &radappiov1alpha3.DeploymentCoordinatorStatus{
		Phase: coordinatorDeleteRunning, ResourceID: outputID, RetryAfter: &metav1.Time{Time: time.Now().Add(-time.Minute)},
		Outputs: []radappiov1alpha3.DeploymentOutputStatus{{ID: outputID, State: outputPendingDelete}},
	}
	output := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "output", Namespace: owner.Namespace}}
	c := newSourceTestClient(t, source, owner, output)
	r := &DeploymentTemplateReconciler{Client: c, Scheme: newFluxPolicyTestScheme(t), EventRecorder: record.NewFakeRecorder(5)}
	request := ctrl.Request{NamespacedName: k8sclient.ObjectKeyFromObject(owner)}
	_, err := r.Reconcile(t.Context(), request)
	require.NoError(t, err)
	var held radappiov1alpha3.DeploymentTemplate
	require.NoError(t, c.Get(t.Context(), request.NamespacedName, &held))
	require.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseHeld, held.Status.Phrase)
	require.Equal(t, coordinatorDeleteRunning, held.Status.Coordinator.Phase)
	require.Equal(t, owner.Status.Coordinator.Outputs, held.Status.Coordinator.Outputs)
	require.Zero(t, c.deletes, "a running Kubernetes delete must not be reissued while held")
	require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(output), output))

	r.SourcePolicy = selectedSourcePolicy(t, c, source)
	_, err = r.Reconcile(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, 1, c.deletes)
	require.True(t, apierrors.IsNotFound(c.Get(t.Context(), k8sclient.ObjectKeyFromObject(output), output)))
}

func Test_DeploymentTemplateReconciler_HoldCompletedDelete(t *testing.T) {
	t.Parallel()
	source := readyFluxSource()
	outputID := "/planes/radius/local/resourceGroups/selected/providers/Applications.Core/containers/output"
	owner := sourceManagedTemplate(t, source)
	owner.Status.OutputResources = []string{outputID}
	owner.Status.Coordinator.Outputs = []radappiov1alpha3.DeploymentOutputStatus{{ID: outputID, State: outputPendingDelete}}
	mock := sdkclients.NewMockResourceDeploymentsClient()
	poller, err := mock.Delete(t.Context(), outputID, "test")
	require.NoError(t, err)
	token, err := poller.ResumeToken()
	require.NoError(t, err)
	mock.CompleteOperation(token, nil)
	owner.Status.Operation = &radappiov1alpha3.ResourceOperation{OperationKind: radappiov1alpha3.OperationKindDelete, ResumeToken: token}
	owner.Status.Coordinator.Phase = coordinatorDeleteRunning
	owner.Status.Coordinator.ResourceID = outputID
	child := &radappiov1alpha3.DeploymentResource{
		ObjectMeta: metav1.ObjectMeta{Name: "output", Namespace: owner.Namespace, UID: "output", Finalizers: []string{DeploymentResourceFinalizer},
			OwnerReferences: []metav1.OwnerReference{{APIVersion: radappiov1alpha3.GroupVersion.String(), Kind: deploymentTemplateKind,
				Name: owner.Name, UID: owner.UID, Controller: new(true)}}},
		Spec: radappiov1alpha3.DeploymentResourceSpec{Id: outputID},
		Status: radappiov1alpha3.DeploymentResourceStatus{
			Phrase: radappiov1alpha3.DeploymentResourcePhraseDeleting, ProvisionedResourceID: outputID,
			Operation: owner.Status.Operation.DeepCopy(),
		},
	}
	c := newSourceTestClient(t, source, owner, child)
	policy, err := NewFluxSourcePolicy(hostoptions.FluxOptions{}, c)
	require.NoError(t, err)
	backend := &sourceTestDeployments{ResourceDeploymentsClient: mock}
	r := &DeploymentTemplateReconciler{Client: c, Scheme: newFluxPolicyTestScheme(t), EventRecorder: record.NewFakeRecorder(5),
		ResourceDeploymentsClient: backend, SourcePolicy: policy}
	request := ctrl.Request{NamespacedName: k8sclient.ObjectKeyFromObject(owner)}

	_, err = r.Reconcile(t.Context(), request)
	require.NoError(t, err)
	var completed radappiov1alpha3.DeploymentTemplate
	require.NoError(t, c.Get(t.Context(), request.NamespacedName, &completed))
	require.Nil(t, completed.Status.Operation)
	require.Equal(t, coordinatorIdle, completed.Status.Coordinator.Phase)
	require.Empty(t, completed.Status.Coordinator.Outputs, "the accepted delete outcome must be durable")
	var tracker radappiov1alpha3.DeploymentResource
	trackerKey := k8sclient.ObjectKeyFromObject(child)
	require.NoError(t, c.Get(t.Context(), trackerKey, &tracker))
	require.Equal(t, radappiov1alpha3.DeploymentResourcePhraseDeleted, tracker.Status.Phrase)
	require.Nil(t, tracker.Status.Operation)
	require.Contains(t, tracker.Finalizers, DeploymentResourceFinalizer, "tracker retirement is a source-authorized effect")

	_, err = r.Reconcile(t.Context(), request)
	require.NoError(t, err)
	var held radappiov1alpha3.DeploymentTemplate
	require.NoError(t, c.Get(t.Context(), request.NamespacedName, &held))
	require.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseHeld, held.Status.Phrase)
	require.NoError(t, c.Get(t.Context(), trackerKey, &tracker))
	require.Contains(t, tracker.Finalizers, DeploymentResourceFinalizer)
	require.Zero(t, c.deletes)

	r.SourcePolicy = selectedSourcePolicy(t, c, source)
	_, err = r.Reconcile(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, 1, c.deletes, "the authorized idle pass retires the completed tracker")
	err = c.Get(t.Context(), trackerKey, &tracker)
	require.True(t, apierrors.IsNotFound(err))
	require.Zero(t, backend.deletes, "a completed backend delete must not be issued again")
}

func Test_DeploymentTemplateReconciler_AttributionOnlyChange(t *testing.T) {
	t.Parallel()
	dt := &radappiov1alpha3.DeploymentTemplate{
		Spec: radappiov1alpha3.DeploymentTemplateSpec{Template: "{}", Repository: "apps"},
	}
	hash, err := computeHash(dt)
	require.NoError(t, err)
	dt.Status.StatusHash = hash
	dt.Spec.SourceRef = sourceReference(readyFluxSource())
	require.True(t, isUpToDate(dt), "associating an unchanged legacy template must not redeploy it")
	dt.Spec.SourceRef.Revision, dt.Spec.SourceRef.Digest = "new revision", "new digest"
	require.True(t, isUpToDate(dt), "new artifact attribution alone must not redeploy")
	dt.Spec.Template = `{"resources":[]}`
	require.False(t, isUpToDate(dt))
}

func Test_DeploymentTemplateReconciler_AttributionOnlyGeneration(t *testing.T) {
	t.Parallel()
	source := readyFluxSource()
	dt := sourceManagedTemplate(t, source)
	hash, err := computeHash(dt)
	require.NoError(t, err)
	dt.Generation, dt.Status.ObservedGeneration, dt.Status.StatusHash = 2, 1, hash
	c := newSourceTestClient(t, source, dt)
	backend := &sourceTestDeployments{ResourceDeploymentsClient: sdkclients.NewMockResourceDeploymentsClient()}
	r := &DeploymentTemplateReconciler{Client: c, Scheme: newFluxPolicyTestScheme(t), EventRecorder: record.NewFakeRecorder(5),
		Radius: NewMockRadiusClient(), ResourceDeploymentsClient: backend, SourcePolicy: selectedSourcePolicy(t, c, source)}
	request := ctrl.Request{NamespacedName: k8sclient.ObjectKeyFromObject(dt)}
	for range 2 {
		_, err = r.Reconcile(t.Context(), request)
		require.NoError(t, err)
	}
	var got radappiov1alpha3.DeploymentTemplate
	require.NoError(t, c.Get(t.Context(), request.NamespacedName, &got))
	require.Zero(t, backend.creates, "an attribution-only change must not redeploy")
	require.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseReady, got.Status.Phrase)
	require.Equal(t, got.Generation, got.Status.ObservedGeneration, "a settled template must observe its current generation")
}
