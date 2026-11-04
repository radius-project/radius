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
	"testing"
	"time"

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	k8sClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	crconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

const (
	DeploymentResourceTestWaitDuration            = time.Second * 10
	DeploymentResourceTestWaitInterval            = time.Millisecond * 200
	DeploymentResourceTestControllerDelayInterval = time.Millisecond * 100

	TestDeploymentResourceNamespace           = "deploymentresource-basic"
	TestDeploymentResourceName                = "test-deploymentresource"
	TestDeploymentResourceRadiusResourceGroup = "default-deploymentresource-basic"
)

var (
	TestDeploymentResourceScope = fmt.Sprintf("/planes/radius/local/resourcegroups/%s", TestDeploymentResourceRadiusResourceGroup)
	TestDeploymentResourceID    = fmt.Sprintf("%s/providers/Microsoft.Resources/deployments/%s", TestDeploymentResourceScope, TestDeploymentResourceName)
)

func SetupDeploymentResourceTest(t *testing.T) (*mockRadiusClient, *sdkclients.MockResourceDeploymentsClient, k8sClient.Client) {
	SkipWithoutEnvironment(t)

	// Shut down the manager when the test exits.
	ctx, cancel := context.WithCancel(t.Context())

	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme: scheme,
		Controller: crconfig.Controller{
			SkipNameValidation: new(true),
		},

		// Suppress metrics in tests to avoid conflicts.
		Metrics: server.Options{
			BindAddress: "0",
		},
	})
	require.NoError(t, err)

	mockRadiusClient := NewMockRadiusClient()
	mockResourceDeploymentsClient := sdkclients.NewMockResourceDeploymentsClient()

	//nolint:staticcheck // SA1019: GetEventRecorderFor is deprecated but migration to new events API requires significant refactoring
	err = (&DeploymentResourceReconciler{
		Client:                    mgr.GetClient(),
		Scheme:                    mgr.GetScheme(),
		EventRecorder:             mgr.GetEventRecorderFor("deploymentresource-controller"),
		Radius:                    mockRadiusClient,
		ResourceDeploymentsClient: mockResourceDeploymentsClient,
		DelayInterval:             DeploymentResourceTestControllerDelayInterval,
		DeleteRetryInterval:       DeploymentResourceTestControllerDelayInterval,
	}).SetupWithManager(mgr)
	require.NoError(t, err)

	startManager(t, mgr, ctx, cancel)

	return mockRadiusClient, mockResourceDeploymentsClient, mgr.GetClient()
}

func Test_DeploymentResourceReconciler_Basic(t *testing.T) {
	ctx := t.Context()
	_, _, k8sClient := SetupDeploymentTemplateTest(t)

	name := types.NamespacedName{Namespace: TestDeploymentResourceNamespace, Name: TestDeploymentResourceName}
	err := k8sClient.Create(ctx, &corev1.Namespace{Name: name.Namespace})
	require.NoError(t, err)

	deployment := makeDeploymentResource(name, TestDeploymentResourceID)
	err = k8sClient.Create(ctx, deployment)
	require.NoError(t, err)

	// Deployment will update after operation completes
	status := waitForDeploymentResourceStateReady(t, k8sClient, name)
	require.Equal(t, TestDeploymentResourceID, status.Id)

	err = k8sClient.Delete(ctx, deployment)
	require.NoError(t, err)

	// Now deleting of the DeploymentResource object can complete.
	waitForDeploymentResourceDeleted(t, k8sClient, name)
}

func Test_DeploymentResourceReconciler_DeleteRetryBackoff(t *testing.T) {
	// Verifies the bounded delete-path retry: a failed UCP delete must
	// trigger a fresh delete attempt within the bounded DeleteRetryInterval
	// rather than controller-runtime's exponential rate-limiter (which
	// climbs to ~16 minutes).

	ctx := t.Context()
	_, mockDeploymentClient, k8sClient := SetupDeploymentResourceTest(t)

	name := types.NamespacedName{Namespace: "deploymentresource-deleteretry", Name: TestDeploymentResourceName}
	err := k8sClient.Create(ctx, &corev1.Namespace{Name: name.Namespace})
	require.NoError(t, err)

	// A finalizer keeps the DR alive past Delete. The DeploymentResource must be controlled by a
	// DeploymentTemplate whose deployment scope covers Spec.Id, otherwise reconcileDelete skips the
	// delete.
	providerConfig, err := sdkclients.NewDefaultProviderConfig(name.Namespace).String()
	require.NoError(t, err)

	owner := makeDeploymentTemplate(types.NamespacedName{Namespace: name.Namespace, Name: "owner-template"}, "{}", providerConfig, nil)
	err = k8sClient.Create(ctx, owner)
	require.NoError(t, err)

	resourceID := fmt.Sprintf("/planes/radius/local/resourcegroups/%s/providers/Applications.Core/containers/%s", name.Namespace, name.Name)
	deployment := &radappiov1alpha3.DeploymentResource{
		Namespace:  name.Namespace,
		Name:       name.Name,
		Finalizers: []string{DeploymentResourceFinalizer},
		OwnerReferences: []metav1.OwnerReference{
			{
				APIVersion:         radappiov1alpha3.GroupVersion.String(),
				Kind:               deploymentTemplateKind,
				Name:               owner.Name,
				UID:                owner.UID,
				Controller:         to.Ptr(true),
				BlockOwnerDeletion: to.Ptr(true),
			},
		},
		Spec: radappiov1alpha3.DeploymentResourceSpec{Id: resourceID},
	}
	err = k8sClient.Create(ctx, deployment)
	require.NoError(t, err)

	_ = waitForDeploymentResourceStateReady(t, k8sClient, name)

	err = k8sClient.Delete(ctx, deployment)
	require.NoError(t, err)

	firstStatus := waitForDeploymentResourceStateDeleting(t, k8sClient, name, nil)
	firstOperation := firstStatus.Operation
	require.NotNil(t, firstOperation, "first delete attempt should have an operation")

	mockDeploymentClient.CompleteOperation(firstOperation.ResumeToken, func(state *sdkclients.OperationState) {
		state.Err = errors.New("transient dependency failure")
	})

	// A fresh delete operation must start within the bounded retry interval.
	// We don't assert on the brief intermediate Failed phrase because the
	// Failed -> reconcileDelete -> Deleting cycle completes faster than the
	// test polling interval (same approach as
	// Test_DeploymentTemplateReconciler_FailureRecovery).
	secondStatus := waitForDeploymentResourceStateDeleting(t, k8sClient, name, firstOperation)
	require.NotNil(t, secondStatus.Operation, "retry should have an operation")
	require.NotEqual(t, firstOperation.ResumeToken, secondStatus.Operation.ResumeToken, "retry should start a fresh operation")

	mockDeploymentClient.CompleteOperation(secondStatus.Operation.ResumeToken, nil)
	waitForDeploymentResourceDeleted(t, k8sClient, name)
}

func Test_DeploymentResourceReconciler_SkipsDeleteOutsideTemplateScope(t *testing.T) {
	// A DeploymentResource whose Spec.Id points outside its owning DeploymentTemplate's deployment
	// scope must NOT trigger a UCP delete: the controller only deletes resources it provisioned
	// within the owning template's scope.

	ctx := t.Context()
	_, mockDeploymentClient, k8sClient := SetupDeploymentResourceTest(t)

	name := types.NamespacedName{Namespace: "deploymentresource-outofscope", Name: "out-of-scope"}
	err := k8sClient.Create(ctx, &corev1.Namespace{Name: name.Namespace})
	require.NoError(t, err)

	// An owning DeploymentTemplate scoped to its own resource group.
	providerConfig, err := sdkclients.NewDefaultProviderConfig(name.Namespace).String()
	require.NoError(t, err)

	owner := makeDeploymentTemplate(types.NamespacedName{Namespace: name.Namespace, Name: "owner-template"}, "{}", providerConfig, nil)
	err = k8sClient.Create(ctx, owner)
	require.NoError(t, err)

	// Spec.Id points at a resource in a DIFFERENT resource group than the owning template's scope.
	outOfScopeID := "/planes/radius/local/resourcegroups/other-group/providers/Applications.Core/environments/default"
	deployment := &radappiov1alpha3.DeploymentResource{
		Namespace:  name.Namespace,
		Name:       name.Name,
		Finalizers: []string{DeploymentResourceFinalizer},
		OwnerReferences: []metav1.OwnerReference{
			{
				APIVersion:         radappiov1alpha3.GroupVersion.String(),
				Kind:               deploymentTemplateKind,
				Name:               owner.Name,
				UID:                owner.UID,
				Controller:         to.Ptr(true),
				BlockOwnerDeletion: to.Ptr(true),
			},
		},
		Spec: radappiov1alpha3.DeploymentResourceSpec{Id: outOfScopeID},
	}
	err = k8sClient.Create(ctx, deployment)
	require.NoError(t, err)

	_ = waitForDeploymentResourceStateReady(t, k8sClient, name)

	err = k8sClient.Delete(ctx, deployment)
	require.NoError(t, err)

	// The object is removed (finalizer dropped) without any UCP delete being issued.
	waitForDeploymentResourceDeleted(t, k8sClient, name)
	require.Empty(t, mockDeploymentClient.DeletedResourceIDs(), "controller must not issue a UCP delete for a resource outside the owning template's scope")
}

func Test_resourceWithinScope(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		resourceID string
		scope      string
		want       bool
		wantErr    bool
	}{
		{
			name:       "resource in scope",
			resourceID: "/planes/radius/local/resourceGroups/my-group/providers/Applications.Core/environments/default",
			scope:      "/planes/radius/local/resourceGroups/my-group",
			want:       true,
		},
		{
			name:       "resource in scope with different casing",
			resourceID: "/planes/radius/local/resourcegroups/my-group/providers/Applications.Core/containers/web",
			scope:      "/planes/radius/local/resourceGroups/my-group",
			want:       true,
		},
		{
			name:       "resource in a different resource group",
			resourceID: "/planes/radius/local/resourceGroups/group-a/providers/Applications.Core/environments/default",
			scope:      "/planes/radius/local/resourceGroups/group-b",
			want:       false,
		},
		{
			name:       "invalid resource id",
			resourceID: "not-a-valid-id",
			scope:      "/planes/radius/local/resourceGroups/my-group",
			wantErr:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := resourceWithinScope(tc.resourceID, tc.scope)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func Test_DeploymentResourceReconciler_KubernetesOutputDelete(t *testing.T) {
	t.Parallel()

	const outputID = "/planes/kubernetes/local/namespaces/workload/providers/core/ConfigMap/inventory-owned"
	cases := []struct {
		name            string
		annotationID    string
		recordedID      string
		inventoryID     string
		resourceID      string
		staleOwner      bool
		missingOwner    bool
		outputFinalizer bool
		outputMissing   bool
		deleteError     bool
		wantDelete      bool
	}{
		{name: "existing child in template inventory", inventoryID: outputID, wantDelete: true},
		{name: "recorded child after removal from inventory", recordedID: outputID, wantDelete: true},
		{name: "forged annotation", annotationID: outputID},
		{name: "untracked Kubernetes resource"},
		{name: "changed resource id", recordedID: outputID, resourceID: "/planes/kubernetes/local/namespaces/workload/providers/core/ConfigMap/other"},
		{name: "changed id still in inventory", recordedID: "/planes/kubernetes/local/namespaces/workload/providers/core/ConfigMap/other", inventoryID: outputID},
		{name: "stale owner", recordedID: outputID, staleOwner: true},
		{name: "missing owner", recordedID: outputID, missingOwner: true},
		{name: "different cluster", inventoryID: "/planes/kubernetes/remote/namespaces/workload/providers/core/ConfigMap/inventory-owned", resourceID: "/planes/kubernetes/remote/namespaces/workload/providers/core/ConfigMap/inventory-owned"},
		{name: "output finalizer", recordedID: outputID, outputFinalizer: true, wantDelete: true},
		{name: "already deleted output", recordedID: outputID, outputMissing: true, wantDelete: true},
		{name: "delete failure", recordedID: outputID, deleteError: true, wantDelete: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			s := runtime.NewScheme()
			require.NoError(t, corev1.AddToScheme(s))
			require.NoError(t, radappiov1alpha3.AddToScheme(s))
			providerConfig, err := sdkclients.NewDefaultProviderConfig("integration").String()
			require.NoError(t, err)
			owner := makeDeploymentTemplate(types.NamespacedName{Namespace: "integration", Name: "inventory"}, "{}", providerConfig, nil)
			owner.UID = "owner-uid"
			if tc.inventoryID != "" {
				owner.Status.OutputResources = []string{tc.inventoryID}
			}
			id := tc.resourceID
			if id == "" {
				id = outputID
			}
			child := makeDeploymentResource(types.NamespacedName{Namespace: owner.Namespace, Name: "inventory-owned"}, id)
			child.Finalizers = []string{DeploymentResourceFinalizer}
			child.Annotations = map[string]string{"radapp.io/deployment-output-id": tc.annotationID}
			child.Status.ProvisionedResourceID = tc.recordedID
			child.OwnerReferences = []metav1.OwnerReference{{
				APIVersion: radappiov1alpha3.GroupVersion.String(),
				Kind:       deploymentTemplateKind,
				Name:       owner.Name,
				UID:        owner.UID,
				Controller: new(true),
			}}
			if tc.staleOwner {
				child.OwnerReferences[0].UID = "stale-uid"
			}
			output := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "workload", Name: "inventory-owned"}}
			if tc.outputFinalizer {
				output.Finalizers = []string{"example.com/hold"}
			}
			mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{corev1.SchemeGroupVersion})
			mapper.Add(corev1.SchemeGroupVersion.WithKind("ConfigMap"), meta.RESTScopeNamespace)
			builder := fake.NewClientBuilder().WithScheme(s).WithRESTMapper(mapper).WithStatusSubresource(child).WithObjects(child)
			if !tc.missingOwner {
				builder.WithObjects(owner)
			}
			if !tc.outputMissing {
				builder.WithObjects(output)
			}
			if tc.deleteError {
				builder.WithInterceptorFuncs(interceptor.Funcs{
					Delete: func(ctx context.Context, c k8sClient.WithWatch, obj k8sClient.Object, opts ...k8sClient.DeleteOption) error {
						if obj.GetObjectKind().GroupVersionKind().Kind == "ConfigMap" {
							return errors.New("delete denied")
						}
						return c.Delete(ctx, obj, opts...)
					},
				})
			}
			c := builder.Build()
			mockDeploymentClient := sdkclients.NewMockResourceDeploymentsClient()
			r := &DeploymentResourceReconciler{
				Client:                    c,
				EventRecorder:             record.NewFakeRecorder(10),
				ResourceDeploymentsClient: mockDeploymentClient,
			}
			result, err := r.reconcileDelete(ctx, child)
			if tc.deleteError {
				require.ErrorContains(t, err, "delete denied")
			} else {
				require.NoError(t, err)
			}
			require.Empty(t, mockDeploymentClient.DeletedResourceIDs(), "Kubernetes outputs must not be sent to UCP")
			actual := &corev1.ConfigMap{}
			err = c.Get(ctx, k8sClient.ObjectKeyFromObject(output), actual)
			if tc.wantDelete && !tc.outputFinalizer && !tc.deleteError {
				require.True(t, apierrors.IsNotFound(err), "Kubernetes output should be deleted: %v", err)
			} else {
				require.NoError(t, err)
			}
			if tc.outputFinalizer {
				require.NotNil(t, actual.DeletionTimestamp)
				require.Positive(t, result.RequeueAfter)
				require.Contains(t, child.Finalizers, DeploymentResourceFinalizer)
				actual.Finalizers = nil
				require.NoError(t, c.Update(ctx, actual))
				_, err = r.reconcileDelete(ctx, child)
				require.NoError(t, err)
			}
			if tc.deleteError {
				require.Contains(t, child.Finalizers, DeploymentResourceFinalizer)
			} else {
				require.NotContains(t, child.Finalizers, DeploymentResourceFinalizer)
			}
		})
	}
}

func Test_DeploymentResourceReconciler_RemovedOutputAPI(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name           string
		discoveryError bool
		apiPresent     bool
	}{
		{name: "removed API"},
		{name: "discovery failure", discoveryError: true},
		{name: "missing output with stale mapping", apiPresent: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			s := runtime.NewScheme()
			require.NoError(t, radappiov1alpha3.AddToScheme(s))
			child := makeDeploymentResource(types.NamespacedName{Namespace: "test", Name: "output"},
				"/planes/kubernetes/local/namespaces/test/providers/example.com/Widget/output")
			child.Finalizers = []string{DeploymentResourceFinalizer}
			c := fake.NewClientBuilder().WithScheme(s).WithObjects(child).Build()
			d := &fakediscovery.FakeDiscovery{Fake: &k8stesting.Fake{}}
			if tc.apiPresent {
				d.Resources = []*metav1.APIResourceList{{
					GroupVersion: "example.com/v1",
					APIResources: []metav1.APIResource{{Name: "widgets", Kind: "Widget"}},
				}}
			}
			if tc.discoveryError {
				d.PrependReactor("get", "resource", func(k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, errors.New("discovery unavailable")
				})
			}
			r := &DeploymentResourceReconciler{Client: c, Discovery: d}
			id, err := resources.ParseResource(child.Spec.Id)
			require.NoError(t, err)
			_, err = r.deleteKubernetesResource(ctx, child, id)
			if tc.discoveryError {
				require.Error(t, err)
				require.Contains(t, child.Finalizers, DeploymentResourceFinalizer)
			} else {
				require.NoError(t, err)
				require.Empty(t, child.Finalizers)
			}
		})
	}
}

func Test_DeploymentResourceReconciler_StaleServedVersion(t *testing.T) {
	t.Parallel()
	for _, discoveryFailure := range []bool{false, true} {
		t.Run(fmt.Sprintf("discovery-failure-%t", discoveryFailure), func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			s := runtime.NewScheme()
			require.NoError(t, radappiov1alpha3.AddToScheme(s))
			oldVersion := schema.GroupVersion{Group: "example.com", Version: "v1"}
			newVersion := schema.GroupVersion{Group: "example.com", Version: "v2"}
			mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{oldVersion, newVersion})
			mapper.Add(oldVersion.WithKind("Widget"), meta.RESTScopeNamespace)
			mapper.Add(newVersion.WithKind("Widget"), meta.RESTScopeNamespace)
			child := makeDeploymentResource(types.NamespacedName{Namespace: "test", Name: "output"},
				"/planes/kubernetes/local/namespaces/test/providers/example.com/Widget/output")
			child.Finalizers = []string{DeploymentResourceFinalizer}
			output := &unstructured.Unstructured{}
			output.SetGroupVersionKind(newVersion.WithKind("Widget"))
			output.SetNamespace("test")
			output.SetName("output")
			output.SetFinalizers([]string{"example.com/hold"})
			c := fake.NewClientBuilder().WithScheme(s).WithRESTMapper(mapper).WithObjects(child, output).
				WithInterceptorFuncs(interceptor.Funcs{
					Delete: func(ctx context.Context, c k8sClient.WithWatch, obj k8sClient.Object, opts ...k8sClient.DeleteOption) error {
						if obj.GetObjectKind().GroupVersionKind() == oldVersion.WithKind("Widget") {
							return apierrors.NewNotFound(oldVersion.WithResource("widgets").GroupResource(), obj.GetName())
						}
						return c.Delete(ctx, obj, opts...)
					},
					Get: func(ctx context.Context, c k8sClient.WithWatch, key k8sClient.ObjectKey, obj k8sClient.Object, opts ...k8sClient.GetOption) error {
						if obj.GetObjectKind().GroupVersionKind() == oldVersion.WithKind("Widget") {
							return apierrors.NewNotFound(oldVersion.WithResource("widgets").GroupResource(), key.Name)
						}
						return c.Get(ctx, key, obj, opts...)
					},
				}).Build()
			d := &fakediscovery.FakeDiscovery{Fake: &k8stesting.Fake{}}
			d.Resources = []*metav1.APIResourceList{{
				GroupVersion: newVersion.String(),
				APIResources: []metav1.APIResource{{Name: "widgets", Kind: "Widget", Namespaced: true}},
			}}
			if discoveryFailure {
				d.PrependReactor("get", "resource", func(k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, errors.New("discovery unavailable")
				})
			}
			r := &DeploymentResourceReconciler{Client: c, Discovery: d}
			id, err := resources.ParseResource(child.Spec.Id)
			require.NoError(t, err)
			result, err := r.deleteKubernetesResource(ctx, child, id)
			if discoveryFailure {
				require.ErrorContains(t, err, "discovery unavailable")
				require.NoError(t, c.Get(ctx, k8sClient.ObjectKeyFromObject(output), output))
				require.Nil(t, output.GetDeletionTimestamp())
			} else {
				require.NoError(t, err)
				require.Positive(t, result.RequeueAfter)
				require.NoError(t, c.Get(ctx, k8sClient.ObjectKeyFromObject(output), output))
				require.NotNil(t, output.GetDeletionTimestamp(), "must delete using the served version, not abandon the output")
				output.SetFinalizers(nil)
				require.NoError(t, c.Update(ctx, output))
				_, err = r.deleteKubernetesResource(ctx, child, id)
				require.NoError(t, err)
				require.Empty(t, child.Finalizers)
				return
			}
			require.Contains(t, child.Finalizers, DeploymentResourceFinalizer)
		})
	}
}

func Test_DeploymentResourceReconciler_NamespaceOutput(t *testing.T) {
	t.Parallel()
	for _, ownNamespace := range []bool{false, true} {
		t.Run(fmt.Sprintf("own-namespace-%t", ownNamespace), func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			s := runtime.NewScheme()
			require.NoError(t, radappiov1alpha3.AddToScheme(s))
			require.NoError(t, corev1.AddToScheme(s))
			namespace := "output"
			if !ownNamespace {
				namespace = "other"
			}
			output := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace, Finalizers: []string{"example.com/hold"}}}
			child := makeDeploymentResource(types.NamespacedName{Namespace: "output", Name: "namespace"},
				"/planes/kubernetes/local/providers/core/Namespace/"+namespace)
			child.Finalizers = []string{DeploymentResourceFinalizer}
			mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{corev1.SchemeGroupVersion})
			mapper.Add(corev1.SchemeGroupVersion.WithKind("Namespace"), meta.RESTScopeRoot)
			c := fake.NewClientBuilder().WithScheme(s).WithRESTMapper(mapper).WithObjects(child, output).Build()
			r := &DeploymentResourceReconciler{Client: c}
			id, err := resources.ParseResource(child.Spec.Id)
			require.NoError(t, err)
			result, err := r.deleteKubernetesResource(ctx, child, id)
			require.NoError(t, err)
			require.NoError(t, c.Get(ctx, k8sClient.ObjectKeyFromObject(output), output))
			require.NotNil(t, output.DeletionTimestamp)
			if ownNamespace {
				require.Empty(t, child.Finalizers, "release child so namespace content deletion can complete")
			} else {
				require.Positive(t, result.RequeueAfter)
				require.Contains(t, child.Finalizers, DeploymentResourceFinalizer, "unrelated namespaces must still drain")
			}
		})
	}
}

func Test_DeploymentResource_OutputIdentityStatusSubresource(t *testing.T) {
	SkipWithoutEnvironment(t)
	ctx := t.Context()
	c, err := k8sClient.New(config, k8sClient.Options{Scheme: scheme})
	require.NoError(t, err)
	namespace := "protected-output-identity"
	require.NoError(t, c.Create(ctx, &corev1.Namespace{Name: namespace}))
	require.NoError(t, c.Create(ctx, &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "resource-editor", Namespace: namespace},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{"radapp.io"},
			Resources: []string{"deploymentresources"},
			Verbs:     []string{"get", "create", "update", "delete"},
		}},
	}))
	require.NoError(t, c.Create(ctx, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "resource-editor", Namespace: namespace},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: "resource-editor"},
		Subjects:   []rbacv1.Subject{{Kind: "User", Name: "deployment-resource-editor"}},
	}))
	callerConfig := rest.CopyConfig(config)
	callerConfig.Impersonate.UserName = "deployment-resource-editor"
	caller, err := k8sClient.New(callerConfig, k8sClient.Options{Scheme: scheme})
	require.NoError(t, err)
	child := makeDeploymentResource(types.NamespacedName{Namespace: namespace, Name: "forged"}, "untrusted")
	child.Status.ProvisionedResourceID = "forged-on-create"
	require.Eventually(t, func() bool {
		err = caller.Create(ctx, child)
		return err == nil
	}, time.Second*10, time.Millisecond*100, "wait for RBAC propagation")
	require.Empty(t, child.Status.ProvisionedResourceID, "normal create must not persist provenance")
	child.Status.ProvisionedResourceID = "forged-on-update"
	require.NoError(t, caller.Update(ctx, child))
	require.Empty(t, child.Status.ProvisionedResourceID, "normal update must not persist provenance")
	child.Status.ProvisionedResourceID = "forged-via-status"
	require.True(t, apierrors.IsForbidden(caller.Status().Update(ctx, child)), "resource access must not confer status access")
	child.Status.ProvisionedResourceID = "controller-recorded"
	require.NoError(t, c.Status().Update(ctx, child))
	child.Status.ProvisionedResourceID = "forged-on-update"
	require.NoError(t, caller.Update(ctx, child))
	require.Equal(t, "controller-recorded", child.Status.ProvisionedResourceID)
}

func waitForDeploymentResourceStateReady(t *testing.T, client k8sClient.Client, name types.NamespacedName) *radappiov1alpha3.DeploymentResourceStatus {
	ctx := t.Context()

	logger := t
	status := &radappiov1alpha3.DeploymentResourceStatus{}
	require.EventuallyWithTf(t, func(t *assert.CollectT) {
		logger.Logf("Fetching DeploymentResource: %+v", name)
		current := &radappiov1alpha3.DeploymentResource{}
		err := client.Get(ctx, name, current)
		require.NoError(t, err)

		status = &current.Status
		logger.Logf("DeploymentResource.Status: %+v", current.Status)
		if assert.Equal(t, radappiov1alpha3.DeploymentResourcePhraseReady, current.Status.Phrase) {
			assert.Empty(t, current.Status.Operation)
		}
	}, DeploymentResourceTestWaitDuration, DeploymentResourceTestWaitInterval, "failed to enter ready state")

	return status
}

func waitForDeploymentResourceStateDeleting(t *testing.T, client k8sClient.Client, name types.NamespacedName, oldOperation *radappiov1alpha3.ResourceOperation) *radappiov1alpha3.DeploymentResourceStatus {
	ctx := t.Context()

	logger := t
	status := &radappiov1alpha3.DeploymentResourceStatus{}
	require.EventuallyWithTf(t, func(t *assert.CollectT) {
		logger.Logf("Fetching DeploymentResource: %+v", name)
		current := &radappiov1alpha3.DeploymentResource{}
		err := client.Get(ctx, name, current)
		assert.NoError(t, err)

		status = &current.Status
		logger.Logf("DeploymentResource.Status: %+v", current.Status)
		assert.Equal(t, status.ObservedGeneration, current.Generation, "Status is not updated")

		if assert.Equal(t, radappiov1alpha3.DeploymentResourcePhraseDeleting, current.Status.Phrase) {
			assert.NotEmpty(t, current.Status.Operation)
			assert.NotEqual(t, oldOperation, current.Status.Operation)
		}
	}, DeploymentResourceTestWaitDuration, DeploymentResourceTestWaitInterval, "failed to enter deleting state")

	return status
}

func waitForDeploymentResourceDeleted(t *testing.T, client k8sClient.Client, name types.NamespacedName) {
	ctx := t.Context()

	logger := t
	require.Eventuallyf(t, func() bool {
		logger.Logf("Fetching DeploymentResource: %+v", name)
		current := &radappiov1alpha3.DeploymentResource{}
		err := client.Get(ctx, name, current)
		if apierrors.IsNotFound(err) {
			return true
		}

		logger.Logf("DeploymentResource.Status: %+v", current.Status)
		return false

	}, DeploymentResourceTestWaitDuration, DeploymentResourceTestWaitInterval, "DeploymentResource still exists")
}
