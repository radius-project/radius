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

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	k8sClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	crconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func Test_DeploymentResourceReconciler_SourceState(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		enabled  bool
		complete bool
		change   func(*radappiov1alpha3.DeploymentTemplate, *sourcev1.GitRepository)
		held     bool
	}{
		{name: "selected", enabled: true},
		{name: "disabled", held: true},
		{name: "suspended", enabled: true, change: func(_ *radappiov1alpha3.DeploymentTemplate, s *sourcev1.GitRepository) { s.Spec.Suspend = true }, held: true},
		{name: "legacy", change: func(dt *radappiov1alpha3.DeploymentTemplate, _ *sourcev1.GitRepository) { dt.Spec.SourceRef = nil }, held: true},
		{name: "direct", change: func(dt *radappiov1alpha3.DeploymentTemplate, _ *sourcev1.GitRepository) {
			dt.Spec.SourceRef = nil
			dt.Spec.Repository = ""
		}},
		{name: "completed while disabled", complete: true, held: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			source := readyFluxSource()
			provider, err := sdkclients.NewDefaultProviderConfig("selected").String()
			require.NoError(t, err)
			owner := &radappiov1alpha3.DeploymentTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "application", Namespace: "target", UID: "template"},
				Spec:       radappiov1alpha3.DeploymentTemplateSpec{Repository: source.Name, SourceRef: sourceReference(source), ProviderConfig: provider},
			}
			if tt.change != nil {
				tt.change(owner, source)
			}
			dr := &radappiov1alpha3.DeploymentResource{
				ObjectMeta: metav1.ObjectMeta{
					Name: "resource", Namespace: owner.Namespace, UID: "resource-uid",
					DeletionTimestamp: new(metav1.Now()), Finalizers: []string{DeploymentResourceFinalizer},
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: radappiov1alpha3.GroupVersion.String(), Kind: deploymentTemplateKind,
						Name: owner.Name, UID: owner.UID, Controller: new(true),
					}},
				},
				Spec: radappiov1alpha3.DeploymentResourceSpec{Id: "/planes/radius/local/resourceGroups/selected/providers/Applications.Core/containers/resource"},
			}
			mock := sdkclients.NewMockResourceDeploymentsClient()
			if tt.complete {
				poller, err := mock.Delete(t.Context(), dr.Spec.Id, "test")
				require.NoError(t, err)
				token, err := poller.ResumeToken()
				require.NoError(t, err)
				mock.CompleteOperation(token, nil)
				dr.Status.Operation = &radappiov1alpha3.ResourceOperation{OperationKind: radappiov1alpha3.OperationKindDelete, ResumeToken: token}
			}
			backend := &sourceTestDeployments{ResourceDeploymentsClient: mock}
			c := &fluxWriteCounter{Client: fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).
				WithStatusSubresource(&radappiov1alpha3.DeploymentResource{}).
				WithObjects(source, owner, dr).Build()}
			r := &DeploymentResourceReconciler{
				Client: c, Scheme: newFluxPolicyTestScheme(t), EventRecorder: record.NewFakeRecorder(5),
				ResourceDeploymentsClient: backend,
			}
			if tt.enabled {
				r.SourcePolicy = selectedSourcePolicy(t, c, source)
			}
			result, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: k8sClient.ObjectKeyFromObject(dr)})
			require.NoError(t, err)
			require.Positive(t, result.RequeueAfter)
			var got radappiov1alpha3.DeploymentResource
			require.NoError(t, c.Get(t.Context(), k8sClient.ObjectKeyFromObject(dr), &got))
			require.Equal(t, dr.Finalizers, got.Finalizers)
			if tt.held {
				require.Equal(t, radappiov1alpha3.DeploymentResourcePhraseHeld, got.Status.Phrase)
				require.Equal(t, dr.Status.Operation, got.Status.Operation)
				require.Zero(t, c.updates)
				require.Zero(t, c.deletes)
				require.Zero(t, backend.deletes)
			} else {
				require.Equal(t, 1, backend.deletes)
				require.NotNil(t, got.Status.Operation)
			}
		})
	}
}

func Test_DeploymentResourceReconciler_LiveDependencies(t *testing.T) {
	t.Parallel()
	provider, err := sdkclients.NewDefaultProviderConfig("selected").String()
	require.NoError(t, err)
	template := &radappiov1alpha3.DeploymentTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "template", Namespace: "target", UID: "template-uid"},
		Spec:       radappiov1alpha3.DeploymentTemplateSpec{ProviderConfig: provider},
	}
	parent := &radappiov1alpha3.DeploymentResource{
		ObjectMeta: metav1.ObjectMeta{Name: "parent", Namespace: template.Namespace, UID: "parent-uid",
			DeletionTimestamp: new(metav1.Now()), Finalizers: []string{DeploymentResourceFinalizer},
			OwnerReferences: []metav1.OwnerReference{{Kind: deploymentTemplateKind, Name: template.Name, UID: template.UID, Controller: new(true)}}},
		Spec: radappiov1alpha3.DeploymentResourceSpec{Id: "/planes/radius/local/resourceGroups/selected/providers/Applications.Core/applications/parent"},
	}
	child := parent.DeepCopy()
	child.Name, child.UID, child.DeletionTimestamp = "child", "child-uid", nil
	child.Spec.Id = "/planes/radius/local/resourceGroups/selected/providers/Applications.Core/containers/child"
	cache := &fluxWriteCounter{Client: fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).
		WithStatusSubresource(&radappiov1alpha3.DeploymentResource{}).WithObjects(template, parent).Build()}
	reader := fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).WithObjects(template, parent, child).Build()
	policy, err := NewFluxSourcePolicy(hostoptions.FluxOptions{}, reader)
	require.NoError(t, err)
	backend := &sourceTestDeployments{ResourceDeploymentsClient: sdkclients.NewMockResourceDeploymentsClient()}
	r := &DeploymentResourceReconciler{Client: cache, SourcePolicy: policy, EventRecorder: record.NewFakeRecorder(5),
		ResourceDeploymentsClient: backend}
	result, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: k8sClient.ObjectKeyFromObject(parent)})
	require.NoError(t, err)
	require.Positive(t, result.RequeueAfter)
	require.Zero(t, backend.deletes, "a child absent from the cache must still block backend parent deletion")
	require.Zero(t, cache.updates)
	var current radappiov1alpha3.DeploymentResource
	require.NoError(t, cache.Get(t.Context(), k8sClient.ObjectKeyFromObject(parent), &current))
	require.Nil(t, current.Status.Operation)
	require.Contains(t, current.Finalizers, DeploymentResourceFinalizer)
}

func Test_DeploymentResourceReconciler_RetiredFinalizer(t *testing.T) {
	t.Parallel()
	for _, completing := range []bool{false, true} {
		name := "already retired"
		if completing {
			name = "completion retires cleanup"
		}
		t.Run(name, func(t *testing.T) {
			provider, err := sdkclients.NewDefaultProviderConfig("selected").String()
			require.NoError(t, err)
			owner := &radappiov1alpha3.DeploymentTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "template", Namespace: "target", UID: "template-uid"},
				Spec:       radappiov1alpha3.DeploymentTemplateSpec{ProviderConfig: provider},
			}
			resource := &radappiov1alpha3.DeploymentResource{
				ObjectMeta: metav1.ObjectMeta{Name: "resource", Namespace: owner.Namespace, UID: "resource-uid",
					DeletionTimestamp: new(metav1.Now()), Finalizers: []string{"test.example/retained"},
					OwnerReferences: []metav1.OwnerReference{{Kind: deploymentTemplateKind, Name: owner.Name, UID: owner.UID, Controller: new(true)}}},
				Spec: radappiov1alpha3.DeploymentResourceSpec{Id: "/planes/radius/local/resourceGroups/selected/providers/Applications.Core/containers/resource"},
			}
			mock := sdkclients.NewMockResourceDeploymentsClient()
			if completing {
				resource.Finalizers = append(resource.Finalizers, DeploymentResourceFinalizer)
				poller, err := mock.Delete(t.Context(), resource.Spec.Id, "test")
				require.NoError(t, err)
				token, err := poller.ResumeToken()
				require.NoError(t, err)
				mock.CompleteOperation(token, nil)
				resource.Status.Operation = &radappiov1alpha3.ResourceOperation{OperationKind: radappiov1alpha3.OperationKindDelete, ResumeToken: token}
			}
			c := fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).
				WithStatusSubresource(&radappiov1alpha3.DeploymentResource{}).WithObjects(owner, resource).Build()
			backend := &sourceTestDeployments{ResourceDeploymentsClient: mock}
			r := &DeploymentResourceReconciler{Client: c, ResourceDeploymentsClient: backend, EventRecorder: record.NewFakeRecorder(5)}
			request := ctrl.Request{NamespacedName: k8sClient.ObjectKeyFromObject(resource)}
			for range 2 {
				result, err := r.Reconcile(t.Context(), request)
				require.NoError(t, err)
				require.True(t, result.IsZero())
			}
			var remaining radappiov1alpha3.DeploymentResource
			require.NoError(t, c.Get(t.Context(), request.NamespacedName, &remaining))
			require.Equal(t, []string{"test.example/retained"}, remaining.Finalizers)
			require.Zero(t, backend.deletes, "an absent cleanup finalizer must not submit or resubmit DELETE")
		})
	}
}

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
