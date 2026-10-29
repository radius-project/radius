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
	"os"
	"path"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armdeployments/v3"
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
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	crconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

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

func Test_DeploymentTemplateReconciler_SourceState(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		enabled bool
		change  func(*radappiov1alpha3.DeploymentTemplate, *sourcev1.GitRepository)
		reason  string
	}{
		{name: "selected", enabled: true},
		{name: "disabled", reason: "SourceNotSelected"},
		{name: "legacy", change: func(dt *radappiov1alpha3.DeploymentTemplate, _ *sourcev1.GitRepository) { dt.Spec.SourceRef = nil }, reason: "TemplateSourceRequired"},
		{name: "stale generation", enabled: true, change: func(dt *radappiov1alpha3.DeploymentTemplate, _ *sourcev1.GitRepository) {
			dt.Spec.SourceRef.Generation++
		}, reason: "SourceIdentityChanged"},
		{name: "suspended", enabled: true, change: func(_ *radappiov1alpha3.DeploymentTemplate, s *sourcev1.GitRepository) { s.Spec.Suspend = true }, reason: "SourceNotReady"},
		{name: "inconsistent", enabled: true, change: func(dt *radappiov1alpha3.DeploymentTemplate, _ *sourcev1.GitRepository) {
			dt.Spec.Repository = "different"
		}, reason: "TemplateSourceRequired"},
		{name: "trusted direct", change: func(dt *radappiov1alpha3.DeploymentTemplate, _ *sourcev1.GitRepository) {
			dt.Spec.Repository = ""
			dt.Spec.SourceRef = nil
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			source := readyFluxSource()
			provider, err := sdkclients.NewDefaultProviderConfig("selected").String()
			require.NoError(t, err)
			dt := &radappiov1alpha3.DeploymentTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "application", Namespace: "target", UID: "template", Finalizers: []string{DeploymentTemplateFinalizer}},
				Spec:       radappiov1alpha3.DeploymentTemplateSpec{Template: `{"resources":[]}`, Parameters: map[string]string{"secret": "never-log-this-value"}, ProviderConfig: provider, Repository: source.Name, SourceRef: sourceReference(source)},
				Status:     radappiov1alpha3.DeploymentTemplateStatus{Phrase: radappiov1alpha3.DeploymentTemplatePhraseReady},
			}
			if tt.change != nil {
				tt.change(dt, source)
			}
			c := &fluxWriteCounter{Client: fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).
				WithStatusSubresource(&radappiov1alpha3.DeploymentTemplate{}).WithObjects(source, dt).Build()}
			backend := &sourceTestDeployments{ResourceDeploymentsClient: sdkclients.NewMockResourceDeploymentsClient()}
			radius := &sourceTestRadius{RadiusClient: NewMockRadiusClient()}
			events := record.NewFakeRecorder(10)
			r := &DeploymentTemplateReconciler{Client: c, Scheme: newFluxPolicyTestScheme(t), EventRecorder: events, Radius: radius, ResourceDeploymentsClient: backend}
			if tt.enabled {
				r.SourcePolicy = selectedSourcePolicy(t, c, source)
			}
			result, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: k8sclient.ObjectKeyFromObject(dt)})
			require.NoError(t, err)
			require.Positive(t, result.RequeueAfter)
			var got radappiov1alpha3.DeploymentTemplate
			require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(dt), &got))
			require.Equal(t, dt.Spec, got.Spec)
			require.Equal(t, dt.Finalizers, got.Finalizers)
			if tt.reason != "" {
				require.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseHeld, got.Status.Phrase)
				require.Zero(t, backend.creates)
				require.Zero(t, backend.deletes)
				require.Zero(t, radius.groupCalls)
				require.Zero(t, c.creates)
				require.Zero(t, c.updates)
				require.Zero(t, c.deletes)
				select {
				case event := <-events.Events:
					require.Contains(t, event, tt.reason)
					require.NotContains(t, event, "never-log-this-value")
				default:
					t.Fatal("missing hold diagnostic")
				}
				_, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: k8sclient.ObjectKeyFromObject(dt)})
				require.NoError(t, err)
				require.Empty(t, events.Events)
			} else {
				require.Equal(t, 1, backend.creates)
				require.Positive(t, radius.groupCalls)
				require.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseUpdating, got.Status.Phrase)
				require.NotEmpty(t, got.Status.OperationSpecHash)
			}
		})
	}
}

func Test_DeploymentTemplateReconciler_HoldCompletedOperation(t *testing.T) {
	t.Parallel()
	source := readyFluxSource()
	provider, err := sdkclients.NewDefaultProviderConfig("selected").String()
	require.NoError(t, err)
	dt := &radappiov1alpha3.DeploymentTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "application", Namespace: "target", UID: "template", Finalizers: []string{DeploymentTemplateFinalizer}},
		Spec:       radappiov1alpha3.DeploymentTemplateSpec{Template: `{"resources":[]}`, ProviderConfig: provider, Repository: source.Name, SourceRef: sourceReference(source)},
	}
	submittedHash, err := computeHash(dt)
	require.NoError(t, err)
	mock := sdkclients.NewMockResourceDeploymentsClient()
	poller, err := mock.CreateOrUpdate(t.Context(), sdkclients.Deployment{}, "/planes/radius/local/resourceGroups/selected/providers/Microsoft.Resources/deployments/pending", "test")
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
	dt.Status.OperationSpecHash = submittedHash
	c := &fluxWriteCounter{Client: fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).
		WithStatusSubresource(&radappiov1alpha3.DeploymentTemplate{}).WithObjects(source, dt).Build()}
	backend := &sourceTestDeployments{ResourceDeploymentsClient: mock}
	r := &DeploymentTemplateReconciler{Client: c, Scheme: newFluxPolicyTestScheme(t), EventRecorder: record.NewFakeRecorder(5), ResourceDeploymentsClient: backend}
	_, err = r.Reconcile(t.Context(), ctrl.Request{NamespacedName: k8sclient.ObjectKeyFromObject(dt)})
	require.NoError(t, err)
	var held radappiov1alpha3.DeploymentTemplate
	require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(dt), &held))
	require.Equal(t, token, held.Status.Operation.ResumeToken)
	require.Equal(t, submittedHash, held.Status.OperationSpecHash)
	require.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseHeld, held.Status.Phrase)
	require.Zero(t, c.creates)
	require.Zero(t, c.deletes)
	require.Zero(t, backend.creates)
	require.Zero(t, backend.deletes)
	r.SourcePolicy = selectedSourcePolicy(t, c, source)
	held.Spec.Parameters = map[string]string{"newer": "desired"}
	require.NoError(t, c.Update(t.Context(), &held))
	_, err = r.reconcileOperation(t.Context(), &held)
	require.NoError(t, err)
	require.Nil(t, held.Status.Operation)
	require.Equal(t, submittedHash, held.Status.StatusHash)
	require.False(t, isUpToDate(&held), "completing an old operation must not claim the newer spec executed")
	require.Equal(t, 1, c.creates)
	require.Zero(t, backend.creates)
}

func Test_DeploymentTemplateReconciler_CompletionGeneration(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name          string
		changed       bool
		submittedHash bool
	}{
		{name: "unchanged legacy operation"},
		{name: "changed legacy operation", changed: true},
		{name: "changed recorded operation", changed: true, submittedHash: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dt := &radappiov1alpha3.DeploymentTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "template", Namespace: "target", UID: "template-uid", Generation: 1},
				Spec:       radappiov1alpha3.DeploymentTemplateSpec{Template: "{}"},
				Status:     radappiov1alpha3.DeploymentTemplateStatus{ObservedGeneration: 1},
			}
			originalHash, err := computeHash(dt)
			require.NoError(t, err)
			if tt.submittedHash {
				dt.Status.OperationSpecHash = originalHash
			}
			if tt.changed {
				dt.Generation++
				dt.Spec.Template = `{"resources":[]}`
			}
			backend := sdkclients.NewMockResourceDeploymentsClient()
			poller, err := backend.CreateOrUpdate(t.Context(), sdkclients.Deployment{}, "completed", "test")
			require.NoError(t, err)
			token, err := poller.ResumeToken()
			require.NoError(t, err)
			backend.CompleteOperation(token, nil)
			dt.Status.Operation = &radappiov1alpha3.ResourceOperation{OperationKind: radappiov1alpha3.OperationKindPut, ResumeToken: token}
			c := fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).
				WithStatusSubresource(&radappiov1alpha3.DeploymentTemplate{}).WithObjects(dt).Build()
			r := &DeploymentTemplateReconciler{Client: c, ResourceDeploymentsClient: backend}
			_, err = r.reconcileOperation(t.Context(), dt)
			require.NoError(t, err)
			require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(dt), dt))
			require.Nil(t, dt.Status.Operation)
			require.Equal(t, !tt.changed, isUpToDate(dt))
			if tt.changed && !tt.submittedHash {
				require.Empty(t, dt.Status.StatusHash, "legacy completion must not mark an unsubmitted spec as deployed")
			} else {
				require.Equal(t, originalHash, dt.Status.StatusHash)
			}
		})
	}
}

func Test_DeploymentTemplateReconciler_HoldDeletion(t *testing.T) {
	t.Parallel()
	source := readyFluxSource()
	owner := &radappiov1alpha3.DeploymentTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "application", Namespace: "target", UID: "template",
			DeletionTimestamp: new(metav1.Now()), Finalizers: []string{DeploymentTemplateFinalizer}},
		Spec: radappiov1alpha3.DeploymentTemplateSpec{Repository: source.Name, SourceRef: sourceReference(source)},
	}
	child := &radappiov1alpha3.DeploymentResource{
		ObjectMeta: metav1.ObjectMeta{Name: "output", Namespace: owner.Namespace, UID: "output",
			Finalizers:      []string{DeploymentResourceFinalizer},
			OwnerReferences: []metav1.OwnerReference{{Name: owner.Name, UID: owner.UID, Kind: deploymentTemplateKind, Controller: new(true)}}},
	}
	c := &fluxWriteCounter{Client: fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).
		WithStatusSubresource(&radappiov1alpha3.DeploymentTemplate{}).WithObjects(source, owner, child).Build()}
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
	require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(child), &radappiov1alpha3.DeploymentResource{}))
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

func Test_DeploymentTemplateReconciler_ResumeOutputTracking(t *testing.T) {
	t.Parallel()
	for _, sameOwner := range []bool{true, false} {
		name := "matching output"
		if !sameOwner {
			name = "different owner"
		}
		t.Run(name, func(t *testing.T) {
			source := readyFluxSource()
			dt := &radappiov1alpha3.DeploymentTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "application", Namespace: "target", UID: "template"},
				Spec:       radappiov1alpha3.DeploymentTemplateSpec{Template: "{}", Repository: source.Name, SourceRef: sourceReference(source)},
			}
			outputID := "/planes/radius/local/resourceGroups/selected/providers/Applications.Core/containers/output"
			ownerUID := dt.UID
			if !sameOwner {
				ownerUID = "other-template"
			}
			existing := &radappiov1alpha3.DeploymentResource{
				ObjectMeta: metav1.ObjectMeta{Name: "output", Namespace: dt.Namespace, UID: "tracked",
					OwnerReferences: []metav1.OwnerReference{{Kind: deploymentTemplateKind, Name: dt.Name, UID: ownerUID, Controller: new(true)}}},
				Spec: radappiov1alpha3.DeploymentResourceSpec{Id: outputID},
			}
			mock := sdkclients.NewMockResourceDeploymentsClient()
			poller, err := mock.CreateOrUpdate(t.Context(), sdkclients.Deployment{}, "pending", "test")
			require.NoError(t, err)
			token, err := poller.ResumeToken()
			require.NoError(t, err)
			mock.CompleteOperation(token, func(state *sdkclients.OperationState) {
				state.Value = sdkclients.ClientCreateOrUpdateResponse{Properties: &armdeployments.DeploymentPropertiesExtended{
					OutputResources: []*armdeployments.ResourceReference{{ID: new(outputID)}},
				}}
			})
			dt.Status.Operation = &radappiov1alpha3.ResourceOperation{OperationKind: radappiov1alpha3.OperationKindPut, ResumeToken: token}
			c := fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).
				WithStatusSubresource(&radappiov1alpha3.DeploymentTemplate{}).WithObjects(source, dt, existing).Build()
			r := &DeploymentTemplateReconciler{Client: c, Scheme: newFluxPolicyTestScheme(t), ResourceDeploymentsClient: mock, SourcePolicy: selectedSourcePolicy(t, c, source)}
			_, err = r.reconcileOperation(t.Context(), dt)
			if sameOwner {
				require.NoError(t, err)
				require.Nil(t, dt.Status.Operation)
			} else {
				require.ErrorContains(t, err, "OutputResourceMismatch")
				require.NotNil(t, dt.Status.Operation)
			}
			var got radappiov1alpha3.DeploymentResource
			require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(existing), &got))
			require.Equal(t, existing.UID, got.UID)
			require.Equal(t, existing.OwnerReferences, got.OwnerReferences)
		})
	}
}

func Test_DeploymentTemplateReconciler_PendingCleanup(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name            string
		wanted          bool
		deleting        bool
		deleteOperation bool
		differentOwner  bool
		pending         bool
		reapply         bool
	}{
		{name: "healthy desired", wanted: true},
		{name: "residual", pending: true},
		{name: "terminating desired", wanted: true, deleting: true, pending: true, reapply: true},
		{name: "accepted delete", wanted: true, deleteOperation: true, pending: true, reapply: true},
		{name: "different owner", wanted: true, deleting: true, differentOwner: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dt := &radappiov1alpha3.DeploymentTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "application", Namespace: "target", UID: "template"},
			}
			dr := &radappiov1alpha3.DeploymentResource{
				ObjectMeta: metav1.ObjectMeta{Name: "output", Namespace: dt.Namespace, Finalizers: []string{DeploymentResourceFinalizer},
					OwnerReferences: []metav1.OwnerReference{{Kind: deploymentTemplateKind, Name: dt.Name, UID: dt.UID, Controller: new(true)}}},
				Spec: radappiov1alpha3.DeploymentResourceSpec{Id: "/planes/radius/local/resourceGroups/selected/providers/Applications.Core/containers/output"},
			}
			if tt.wanted {
				dt.Status.OutputResources = []string{dr.Spec.Id}
			}
			if tt.deleting {
				dr.DeletionTimestamp = new(metav1.Now())
			}
			if tt.deleteOperation {
				dr.Status.Operation = &radappiov1alpha3.ResourceOperation{OperationKind: radappiov1alpha3.OperationKindDelete}
			}
			if tt.differentOwner {
				dr.OwnerReferences[0].UID = "different-template"
			}
			cache := fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).WithObjects(dt).Build()
			reader := fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).WithObjects(dt, dr).Build()
			policy, err := NewFluxSourcePolicy(hostoptions.FluxOptions{}, reader)
			require.NoError(t, err)
			r := &DeploymentTemplateReconciler{Client: cache, SourcePolicy: policy}
			cleanup, err := r.pendingDeploymentResourceCleanup(t.Context(), dt, dt.Status.OutputResources)
			require.NoError(t, err)
			require.Equal(t, tt.pending, cleanup.pending, "the live reader must observe resources absent from the cache")
			require.Equal(t, tt.reapply, cleanup.desiredDeleting)
			require.Equal(t, tt.pending, cleanup.blocksUpdate)
		})
	}
}

func Test_DeploymentTemplateReconciler_DesiredOutputDeletion(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name           string
		putCompleted   bool
		outputRecorded bool
		upToDate       bool
	}{
		{name: "wait before PUT"},
		{name: "completed PUT with new output", putCompleted: true},
		{name: "completed PUT with recorded output", putCompleted: true, outputRecorded: true},
		{name: "previously settled output", outputRecorded: true, upToDate: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			source := readyFluxSource()
			provider, err := sdkclients.NewDefaultProviderConfig("selected").String()
			require.NoError(t, err)
			dt := &radappiov1alpha3.DeploymentTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "application", Namespace: "target", UID: "template",
					Finalizers: []string{DeploymentTemplateFinalizer}},
				Spec: radappiov1alpha3.DeploymentTemplateSpec{Template: `{"resources":[]}`, ProviderConfig: provider,
					Repository: source.Name, SourceRef: sourceReference(source)},
				Status: radappiov1alpha3.DeploymentTemplateStatus{Phrase: radappiov1alpha3.DeploymentTemplatePhraseReadyPendingCleanup},
			}
			dt.Status.StatusHash, err = computeHash(dt)
			require.NoError(t, err)
			if !tt.upToDate {
				dt.Spec.Parameters = map[string]string{"revision": "new"}
			}
			outputID := "/planes/radius/local/resourceGroups/selected/providers/Applications.Core/containers/output"
			if tt.outputRecorded {
				dt.Status.OutputResources = []string{outputID}
			}
			mock := sdkclients.NewMockResourceDeploymentsClient()
			deletePoller, err := mock.Delete(t.Context(), outputID, "test")
			require.NoError(t, err)
			deleteToken, err := deletePoller.ResumeToken()
			require.NoError(t, err)
			child := &radappiov1alpha3.DeploymentResource{
				ObjectMeta: metav1.ObjectMeta{Name: "output", Namespace: dt.Namespace, UID: "old-output",
					DeletionTimestamp: new(metav1.Now()), Finalizers: []string{DeploymentResourceFinalizer},
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: radappiov1alpha3.GroupVersion.String(), Kind: deploymentTemplateKind,
						Name: dt.Name, UID: dt.UID, Controller: new(true),
					}}},
				Spec: radappiov1alpha3.DeploymentResourceSpec{Id: outputID},
				Status: radappiov1alpha3.DeploymentResourceStatus{
					Phrase:    radappiov1alpha3.DeploymentResourcePhraseDeleting,
					Operation: &radappiov1alpha3.ResourceOperation{OperationKind: radappiov1alpha3.OperationKindDelete, ResumeToken: deleteToken},
				},
			}
			completePut := func(state *sdkclients.OperationState) {
				state.Value = sdkclients.ClientCreateOrUpdateResponse{Properties: &armdeployments.DeploymentPropertiesExtended{
					OutputResources: []*armdeployments.ResourceReference{{ID: new(outputID)}},
				}}
			}
			if tt.putCompleted {
				poller, err := mock.CreateOrUpdate(t.Context(), sdkclients.Deployment{}, "pending", "test")
				require.NoError(t, err)
				token, err := poller.ResumeToken()
				require.NoError(t, err)
				mock.CompleteOperation(token, completePut)
				dt.Status.Operation = &radappiov1alpha3.ResourceOperation{OperationKind: radappiov1alpha3.OperationKindPut, ResumeToken: token}
				dt.Status.OperationSpecHash, err = computeHash(dt)
				require.NoError(t, err)
			}
			c := fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).
				WithStatusSubresource(&radappiov1alpha3.DeploymentTemplate{}, &radappiov1alpha3.DeploymentResource{}).
				WithObjects(source, dt, child).Build()
			policy := selectedSourcePolicy(t, c, source)
			backend := &sourceTestDeployments{ResourceDeploymentsClient: mock}
			r := &DeploymentTemplateReconciler{Client: c, Scheme: newFluxPolicyTestScheme(t), SourcePolicy: policy,
				EventRecorder: record.NewFakeRecorder(10), Radius: NewMockRadiusClient(), ResourceDeploymentsClient: backend}
			req := ctrl.Request{NamespacedName: k8sclient.ObjectKeyFromObject(dt)}
			result, err := r.Reconcile(t.Context(), req)
			require.NoError(t, err)
			require.Zero(t, backend.creates, "a desired PUT must wait for the old DELETE")
			require.Positive(t, result.RequeueAfter)
			var waiting radappiov1alpha3.DeploymentTemplate
			require.NoError(t, c.Get(t.Context(), req.NamespacedName, &waiting))
			require.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseReadyPendingCleanup, waiting.Status.Phrase)
			require.Nil(t, waiting.Status.Operation)
			require.False(t, isUpToDate(&waiting), "desired outputs must be reapplied after the overlapping DELETE")
			if tt.putCompleted {
				require.Equal(t, []string{outputID}, waiting.Status.OutputResources)
			}

			mock.CompleteOperation(deleteToken, nil)
			resourceController := &DeploymentResourceReconciler{Client: c, SourcePolicy: policy,
				EventRecorder: record.NewFakeRecorder(10), ResourceDeploymentsClient: backend}
			_, err = resourceController.Reconcile(t.Context(), ctrl.Request{NamespacedName: k8sclient.ObjectKeyFromObject(child)})
			require.NoError(t, k8sclient.IgnoreNotFound(err))
			require.True(t, apierrors.IsNotFound(c.Get(t.Context(), k8sclient.ObjectKeyFromObject(child), &radappiov1alpha3.DeploymentResource{})))
			_, err = r.Reconcile(t.Context(), req)
			require.NoError(t, err)
			require.Equal(t, 1, backend.creates)
			require.NoError(t, c.Get(t.Context(), req.NamespacedName, &waiting))
			require.NotNil(t, waiting.Status.Operation)
			require.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseUpdating, waiting.Status.Phrase)

			mock.CompleteOperation(waiting.Status.Operation.ResumeToken, completePut)
			_, err = r.Reconcile(t.Context(), req)
			require.NoError(t, err)
			var ready radappiov1alpha3.DeploymentTemplate
			require.NoError(t, c.Get(t.Context(), req.NamespacedName, &ready))
			require.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseReady, ready.Status.Phrase)
			require.True(t, isUpToDate(&ready))
			require.Equal(t, []string{outputID}, ready.Status.OutputResources)
			var restored radappiov1alpha3.DeploymentResource
			require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(child), &restored))
			require.Nil(t, restored.DeletionTimestamp)
			require.NotEqual(t, child.UID, restored.UID)
			require.Equal(t, outputID, restored.Spec.Id)
			require.True(t, isOwnedBy(restored, &ready))
			require.Contains(t, restored.Finalizers, DeploymentResourceFinalizer)
			require.Equal(t, 1, backend.creates)
			require.Zero(t, backend.deletes, "only the previously accepted DELETE may complete")
		})
	}
}

func Test_DeploymentTemplateReconciler_DependencyBlockedCleanup(t *testing.T) {
	t.Parallel()
	for _, parentType := range []string{"applications", "environments"} {
		for _, tt := range []struct {
			name                string
			restoreParent       bool
			keepChild           bool
			direct              bool
			deleteDuringRestore bool
			pruneOwner          bool
		}{
			{name: "remove all"},
			{name: "restore parent", restoreParent: true},
			{name: "restore parent and retain child", restoreParent: true, keepChild: true},
			{name: "direct remove all", direct: true},
			{name: "delete during restoration", restoreParent: true, keepChild: true, deleteDuringRestore: true},
			{name: "prune during restoration", restoreParent: true, deleteDuringRestore: true, pruneOwner: true},
			{name: "direct delete during restoration", restoreParent: true, keepChild: true, direct: true, deleteDuringRestore: true},
		} {
			t.Run(parentType+"/"+tt.name, func(t *testing.T) {
				source := readyFluxSource()
				provider, err := sdkclients.NewDefaultProviderConfig("selected").String()
				require.NoError(t, err)
				dt := &radappiov1alpha3.DeploymentTemplate{
					ObjectMeta: metav1.ObjectMeta{Name: "template", Namespace: "target", UID: "template-uid", Generation: 1,
						Finalizers: []string{DeploymentTemplateFinalizer}},
					Spec: radappiov1alpha3.DeploymentTemplateSpec{Template: `{"resources":[]}`, ProviderConfig: provider,
						Repository: source.Name, SourceRef: sourceReference(source)},
					Status: radappiov1alpha3.DeploymentTemplateStatus{Phrase: radappiov1alpha3.DeploymentTemplatePhraseReady},
				}
				if tt.direct {
					dt.Spec.Repository, dt.Spec.SourceRef = "", nil
				}
				parentID := "/planes/radius/local/resourceGroups/selected/providers/Applications.Core/" + parentType + "/parent"
				childID := "/planes/radius/local/resourceGroups/selected/providers/Applications.Core/containers/child"
				dt.Status.OutputResources = []string{parentID, childID}
				dt.Status.StatusHash, err = computeHash(dt)
				require.NoError(t, err)
				owner := metav1.OwnerReference{APIVersion: radappiov1alpha3.GroupVersion.String(), Kind: deploymentTemplateKind,
					Name: dt.Name, UID: dt.UID, Controller: new(true)}
				parent := &radappiov1alpha3.DeploymentResource{
					ObjectMeta: metav1.ObjectMeta{Name: "parent", Namespace: dt.Namespace, UID: "parent-uid",
						Finalizers: []string{DeploymentResourceFinalizer}, OwnerReferences: []metav1.OwnerReference{owner}},
					Spec: radappiov1alpha3.DeploymentResourceSpec{Id: parentID},
				}
				child := parent.DeepCopy()
				child.Name, child.UID, child.Spec.Id = "child", "child-uid", childID
				c := fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).
					WithStatusSubresource(&radappiov1alpha3.DeploymentTemplate{}, &radappiov1alpha3.DeploymentResource{}).
					WithObjects(source, dt, parent, child).Build()
				policy := selectedSourcePolicy(t, c, source)
				if tt.direct {
					policy, err = NewFluxSourcePolicy(hostoptions.FluxOptions{}, c)
					require.NoError(t, err)
				}
				mock := sdkclients.NewMockResourceDeploymentsClient()
				backend := &sourceTestDeployments{ResourceDeploymentsClient: mock}
				r := &DeploymentTemplateReconciler{Client: c, Scheme: newFluxPolicyTestScheme(t), SourcePolicy: policy,
					EventRecorder: record.NewFakeRecorder(20), Radius: NewMockRadiusClient(), ResourceDeploymentsClient: backend}
				resourceController := &DeploymentResourceReconciler{Client: c, SourcePolicy: policy,
					EventRecorder: record.NewFakeRecorder(20), ResourceDeploymentsClient: backend}
				request := ctrl.Request{NamespacedName: k8sclient.ObjectKeyFromObject(dt)}
				reconcile := func() {
					t.Helper()
					_, err := r.Reconcile(t.Context(), request)
					require.NoError(t, err)
					require.NoError(t, c.Get(t.Context(), request.NamespacedName, dt))
				}
				completePut := func(outputs []string) {
					t.Helper()
					require.NotNil(t, dt.Status.Operation)
					mock.CompleteOperation(dt.Status.Operation.ResumeToken, func(state *sdkclients.OperationState) {
						refs := make([]*armdeployments.ResourceReference, 0, len(outputs))
						for _, id := range outputs {
							refs = append(refs, &armdeployments.ResourceReference{ID: new(id)})
						}
						state.Value = sdkclients.ClientCreateOrUpdateResponse{Properties: &armdeployments.DeploymentPropertiesExtended{OutputResources: refs}}
					})
				}
				finishPut := func(outputs []string) {
					t.Helper()
					completePut(outputs)
					for range 3 {
						reconcile()
						if dt.Status.Operation == nil {
							return
						}
					}
					t.Fatal("completed deployment did not finish output tracking")
				}
				finishDelete := func(resource *radappiov1alpha3.DeploymentResource) {
					t.Helper()
					key := k8sclient.ObjectKeyFromObject(resource)
					_, err := resourceController.Reconcile(t.Context(), ctrl.Request{NamespacedName: key})
					require.NoError(t, err)
					var pending radappiov1alpha3.DeploymentResource
					require.NoError(t, c.Get(t.Context(), key, &pending))
					require.NotNil(t, pending.Status.Operation)
					mock.CompleteOperation(pending.Status.Operation.ResumeToken, nil)
					_, err = resourceController.Reconcile(t.Context(), ctrl.Request{NamespacedName: key})
					require.NoError(t, k8sclient.IgnoreNotFound(err))
					require.True(t, apierrors.IsNotFound(c.Get(t.Context(), key, &radappiov1alpha3.DeploymentResource{})))
				}

				dt.Spec.Parameters = map[string]string{"revision": "parent removed"}
				dt.Generation++
				require.NoError(t, c.Update(t.Context(), dt))
				reconcile()
				finishPut([]string{childID})
				require.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseReadyPendingCleanup, dt.Status.Phrase)
				_, err = resourceController.Reconcile(t.Context(), ctrl.Request{NamespacedName: k8sclient.ObjectKeyFromObject(parent)})
				require.NoError(t, err)
				var blocked radappiov1alpha3.DeploymentResource
				require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(parent), &blocked))
				require.NotNil(t, blocked.DeletionTimestamp)
				require.Nil(t, blocked.Status.Operation)
				require.Zero(t, backend.deletes, "the live child must prevent parent deletion")
				reconcile()
				require.Equal(t, 1, backend.creates, "unchanged content must not spin while cleanup waits")

				dt.Spec.Parameters["revision"] = "corrected"
				dt.Generation++
				require.NoError(t, c.Update(t.Context(), dt))
				reconcile()
				require.Equal(t, 2, backend.creates, "a corrective update must break dependency-blocked cleanup")
				outputs := []string{}
				if tt.restoreParent {
					outputs = append(outputs, parentID)
				}
				if tt.keepChild {
					outputs = append(outputs, childID)
				}
				if tt.deleteDuringRestore {
					completePut(outputs)
					reconcile()
					require.NotNil(t, dt.Status.Operation, "the completed PUT must remain recoverable until tracking is restored")
					require.True(t, apierrors.IsNotFound(c.Get(t.Context(), k8sclient.ObjectKeyFromObject(parent), &radappiov1alpha3.DeploymentResource{})))
					require.Zero(t, backend.deletes)
					if tt.pruneOwner {
						source.Generation++
						source.Status.ObservedGeneration = source.Generation
						source.Status.Conditions[0].ObservedGeneration = source.Generation
						require.NoError(t, c.Update(t.Context(), source))
						flux := &FluxController{Client: c, SourcePolicy: policy}
						require.NoError(t, flux.pruneTemplate(t.Context(), request.NamespacedName, sourceReference(source)))
					} else {
						require.NoError(t, c.Delete(t.Context(), dt))
					}
					_, err = r.Reconcile(t.Context(), request)
					require.ErrorContains(t, err, "owned resources")
					require.NoError(t, c.Get(t.Context(), request.NamespacedName, dt))
					require.Nil(t, dt.Status.Operation)
					require.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseDeleting, dt.Status.Phrase)
					finishDelete(child)
					var restored radappiov1alpha3.DeploymentResource
					require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(parent), &restored),
						"owner deletion must recover the parent tracker from the completed PUT")
					require.NotNil(t, restored.DeletionTimestamp)
					require.Contains(t, restored.Finalizers, DeploymentResourceFinalizer)
					require.True(t, isOwnedBy(restored, dt))
					finishDelete(parent)
					_, err = r.Reconcile(t.Context(), request)
					require.NoError(t, err)
					require.True(t, apierrors.IsNotFound(c.Get(t.Context(), request.NamespacedName, &radappiov1alpha3.DeploymentTemplate{})))
					require.ElementsMatch(t, []string{childID, parentID}, mock.DeletedResourceIDs())
					require.Equal(t, 2, backend.creates, "deleting an owner must not submit another PUT")
					return
				}
				finishPut(outputs)
				require.Zero(t, backend.deletes, "completed tracking must not itself submit backend deletes")
				if !tt.keepChild {
					finishDelete(child)
				}
				if !tt.restoreParent {
					finishDelete(parent)
				}
				reconcile()
				require.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseReady, dt.Status.Phrase)
				require.True(t, isUpToDate(dt))
				require.Nil(t, dt.Status.Operation)
				require.ElementsMatch(t, outputs, dt.Status.OutputResources)
				require.Equal(t, types.UID("template-uid"), dt.UID)
				require.Equal(t, 2, backend.creates)
				require.Equal(t, 2-len(outputs), backend.deletes)
				if tt.restoreParent {
					var restored radappiov1alpha3.DeploymentResource
					require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(parent), &restored))
					require.Nil(t, restored.DeletionTimestamp)
					require.NotEqual(t, parent.UID, restored.UID)
					require.True(t, isOwnedBy(restored, dt))
					require.Contains(t, restored.Finalizers, DeploymentResourceFinalizer)
				}
				if tt.keepChild {
					var retained radappiov1alpha3.DeploymentResource
					require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(child), &retained))
					require.Equal(t, child.UID, retained.UID)
					require.Nil(t, retained.DeletionTimestamp)
				}
			})
		}
	}
}

func Test_DeploymentTemplateReconciler_DeleteUsesLiveInventory(t *testing.T) {
	t.Parallel()
	template := &radappiov1alpha3.DeploymentTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "template", Namespace: "target", UID: "template-uid",
			DeletionTimestamp: new(metav1.Now()), Finalizers: []string{DeploymentTemplateFinalizer}},
	}
	output := &radappiov1alpha3.DeploymentResource{
		ObjectMeta: metav1.ObjectMeta{Name: "output", Namespace: template.Namespace, UID: "output-uid",
			Finalizers: []string{DeploymentResourceFinalizer}, OwnerReferences: []metav1.OwnerReference{{
				Kind: deploymentTemplateKind, Name: template.Name, UID: template.UID, Controller: new(true),
			}}},
		Spec: radappiov1alpha3.DeploymentResourceSpec{Id: "/planes/radius/local/resourceGroups/selected/providers/Applications.Core/containers/output"},
	}
	reader := fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).
		WithStatusSubresource(&radappiov1alpha3.DeploymentTemplate{}).WithObjects(template, output).Build()
	cache := interceptor.NewClient(reader, interceptor.Funcs{
		List: func(ctx context.Context, c k8sclient.WithWatch, list k8sclient.ObjectList, opts ...k8sclient.ListOption) error {
			if resources, ok := list.(*radappiov1alpha3.DeploymentResourceList); ok {
				resources.Items = nil
				return nil
			}
			return c.List(ctx, list, opts...)
		},
	})
	policy, err := NewFluxSourcePolicy(hostoptions.FluxOptions{}, reader)
	require.NoError(t, err)
	r := &DeploymentTemplateReconciler{Client: cache, SourcePolicy: policy, EventRecorder: record.NewFakeRecorder(5)}
	_, err = r.Reconcile(t.Context(), ctrl.Request{NamespacedName: k8sclient.ObjectKeyFromObject(template)})
	require.ErrorContains(t, err, "owned resources", "a stale empty cache must not authorize template finalization")
	require.NoError(t, reader.Get(t.Context(), k8sclient.ObjectKeyFromObject(template), template))
	require.Contains(t, template.Finalizers, DeploymentTemplateFinalizer)
	require.NoError(t, reader.Get(t.Context(), k8sclient.ObjectKeyFromObject(output), output))
	require.NotNil(t, output.DeletionTimestamp)
}

func Test_DeploymentTemplateReconciler_DeleteCompletedOutputRecovery(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"missing tracker", "terminating tracker", "dependency-blocked tracker", "retired tracker", "unselected source", "tracker write failure", "different owner"} {
		t.Run(scenario, func(t *testing.T) {
			source := readyFluxSource()
			provider, err := sdkclients.NewDefaultProviderConfig("selected").String()
			require.NoError(t, err)
			dt := &radappiov1alpha3.DeploymentTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "template", Namespace: "target", UID: "template-uid",
					DeletionTimestamp: new(metav1.Now()), Finalizers: []string{DeploymentTemplateFinalizer}},
				Spec: radappiov1alpha3.DeploymentTemplateSpec{Template: "{}", ProviderConfig: provider,
					Repository: source.Name, SourceRef: sourceReference(source)},
			}
			outputID := "/planes/radius/local/resourceGroups/selected/providers/Applications.Core/applications/parent"
			existing := &radappiov1alpha3.DeploymentResource{
				ObjectMeta: metav1.ObjectMeta{Name: "parent", Namespace: dt.Namespace, UID: "old-output",
					DeletionTimestamp: new(metav1.Now()), Finalizers: []string{DeploymentResourceFinalizer},
					OwnerReferences: []metav1.OwnerReference{{Kind: deploymentTemplateKind, Name: dt.Name, UID: dt.UID, Controller: new(true)}}},
				Spec: radappiov1alpha3.DeploymentResourceSpec{Id: outputID},
			}
			objects := []k8sclient.Object{source, dt}
			cleanupKeys := []k8sclient.ObjectKey{k8sclient.ObjectKeyFromObject(existing)}
			cleanupIDs := []string{outputID}
			switch scenario {
			case "terminating tracker":
				objects = append(objects, existing)
			case "dependency-blocked tracker":
				child := existing.DeepCopy()
				child.Name, child.UID, child.DeletionTimestamp = "child", "child-uid", nil
				child.Spec.Id = "/planes/radius/local/resourceGroups/selected/providers/Applications.Core/containers/child"
				objects = append(objects, existing, child)
				dt.Status.OutputResources = []string{child.Spec.Id}
				cleanupKeys = append([]k8sclient.ObjectKey{k8sclient.ObjectKeyFromObject(child)}, cleanupKeys...)
				cleanupIDs = append(cleanupIDs, child.Spec.Id)
			case "retired tracker":
				existing.Finalizers = []string{"test.example/retained"}
				objects = append(objects, existing)
			case "different owner":
				existing.OwnerReferences[0].UID = "other-template"
				existing.DeletionTimestamp, existing.Finalizers = nil, nil
				objects = append(objects, existing)
			}
			mock := sdkclients.NewMockResourceDeploymentsClient()
			poller, err := mock.CreateOrUpdate(t.Context(), sdkclients.Deployment{}, "completed", "test")
			require.NoError(t, err)
			token, err := poller.ResumeToken()
			require.NoError(t, err)
			mock.CompleteOperation(token, func(state *sdkclients.OperationState) {
				state.Value = sdkclients.ClientCreateOrUpdateResponse{Properties: &armdeployments.DeploymentPropertiesExtended{
					OutputResources: []*armdeployments.ResourceReference{{ID: new(outputID)}},
				}}
			})
			dt.Status.Operation = &radappiov1alpha3.ResourceOperation{OperationKind: radappiov1alpha3.OperationKindPut, ResumeToken: token}
			dt.Status.OperationSpecHash = "submitted-spec"
			base := fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).
				WithStatusSubresource(&radappiov1alpha3.DeploymentTemplate{}, &radappiov1alpha3.DeploymentResource{}).WithObjects(objects...).Build()
			var failWrite bool
			c := interceptor.NewClient(base, interceptor.Funcs{
				Create: func(ctx context.Context, c k8sclient.WithWatch, object k8sclient.Object, opts ...k8sclient.CreateOption) error {
					if failWrite {
						return errors.New("tracker write failed")
					}
					return c.Create(ctx, object, opts...)
				},
			})
			policy := selectedSourcePolicy(t, base, source)
			backend := &sourceTestDeployments{ResourceDeploymentsClient: mock}
			r := &DeploymentTemplateReconciler{Client: c, Scheme: newFluxPolicyTestScheme(t), SourcePolicy: policy,
				EventRecorder: record.NewFakeRecorder(10), ResourceDeploymentsClient: backend}
			if scenario == "unselected source" {
				r.SourcePolicy = nil
			}
			failWrite = scenario == "tracker write failure"
			request := ctrl.Request{NamespacedName: k8sclient.ObjectKeyFromObject(dt)}
			result, err := r.Reconcile(t.Context(), request)
			switch scenario {
			case "missing tracker", "terminating tracker", "dependency-blocked tracker":
				require.ErrorContains(t, err, "owned resources")
			default:
				if failWrite {
					require.ErrorContains(t, err, "tracker write failed")
				} else {
					require.NoError(t, err)
					require.Positive(t, result.RequeueAfter)
				}
				require.NoError(t, c.Get(t.Context(), request.NamespacedName, dt))
				require.NotNil(t, dt.Status.Operation)
				require.Equal(t, token, dt.Status.Operation.ResumeToken)
				require.Equal(t, "submitted-spec", dt.Status.OperationSpecHash)
				require.Contains(t, dt.Finalizers, DeploymentTemplateFinalizer)
				require.Zero(t, backend.creates)
				require.Zero(t, backend.deletes)
				if scenario == "unselected source" || failWrite {
					require.True(t, apierrors.IsNotFound(c.Get(t.Context(), k8sclient.ObjectKeyFromObject(existing), &radappiov1alpha3.DeploymentResource{})))
				}
				switch scenario {
				case "retired tracker":
					require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(existing), existing))
					require.Equal(t, []string{"test.example/retained"}, existing.Finalizers)
					existing.Finalizers = nil
					require.NoError(t, c.Update(t.Context(), existing))
				case "different owner":
					require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(existing), existing))
					require.Equal(t, types.UID("other-template"), existing.OwnerReferences[0].UID)
					require.NoError(t, c.Delete(t.Context(), existing))
				}
				failWrite = false
				r.SourcePolicy = policy
				_, err = r.Reconcile(t.Context(), request)
				require.ErrorContains(t, err, "owned resources")
			}
			require.NoError(t, c.Get(t.Context(), request.NamespacedName, dt))
			require.Nil(t, dt.Status.Operation)
			require.Empty(t, dt.Status.OperationSpecHash)
			require.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseDeleting, dt.Status.Phrase)
			var tracked radappiov1alpha3.DeploymentResource
			key := k8sclient.ObjectKeyFromObject(existing)
			require.NoError(t, c.Get(t.Context(), key, &tracked))
			require.NotNil(t, tracked.DeletionTimestamp)
			require.Contains(t, tracked.Finalizers, DeploymentResourceFinalizer)
			require.True(t, isOwnedBy(tracked, dt))
			if scenario == "terminating tracker" || scenario == "dependency-blocked tracker" {
				require.Equal(t, existing.UID, tracked.UID, "deleting owners must not retire another output tracker")
			}
			resources := &DeploymentResourceReconciler{Client: c, SourcePolicy: policy,
				EventRecorder: record.NewFakeRecorder(5), ResourceDeploymentsClient: backend}
			for _, cleanupKey := range cleanupKeys {
				_, err = resources.Reconcile(t.Context(), ctrl.Request{NamespacedName: cleanupKey})
				require.NoError(t, err)
				var pending radappiov1alpha3.DeploymentResource
				require.NoError(t, c.Get(t.Context(), cleanupKey, &pending))
				require.NotNil(t, pending.Status.Operation)
				mock.CompleteOperation(pending.Status.Operation.ResumeToken, nil)
				_, err = resources.Reconcile(t.Context(), ctrl.Request{NamespacedName: cleanupKey})
				require.NoError(t, err)
			}
			_, err = r.Reconcile(t.Context(), request)
			require.NoError(t, err)
			require.True(t, apierrors.IsNotFound(c.Get(t.Context(), request.NamespacedName, &radappiov1alpha3.DeploymentTemplate{})))
			require.ElementsMatch(t, cleanupIDs, mock.DeletedResourceIDs())
			require.Zero(t, backend.creates)
		})
	}
}

func Test_DeploymentTemplateReconciler_DependencyCleanupBarrier(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		change func(*radappiov1alpha3.DeploymentResource, *radappiov1alpha3.DeploymentResource)
		allow  bool
	}{
		{name: "live child", allow: true},
		{name: "child draining", change: func(_, child *radappiov1alpha3.DeploymentResource) { child.DeletionTimestamp = new(metav1.Now()) }},
		{name: "child DELETE accepted", change: func(_, child *radappiov1alpha3.DeploymentResource) {
			child.Status.Operation = &radappiov1alpha3.ResourceOperation{OperationKind: radappiov1alpha3.OperationKindDelete}
		}},
		{name: "parent DELETE accepted", change: func(parent, _ *radappiov1alpha3.DeploymentResource) {
			parent.Status.Operation = &radappiov1alpha3.ResourceOperation{OperationKind: radappiov1alpha3.OperationKindDelete}
		}},
		{name: "parent starting DELETE", change: func(parent, _ *radappiov1alpha3.DeploymentResource) {
			parent.Status.Phrase = radappiov1alpha3.DeploymentResourcePhraseDeleting
		}},
		{name: "parent DELETE failed", change: func(parent, _ *radappiov1alpha3.DeploymentResource) {
			parent.Status.Phrase = radappiov1alpha3.DeploymentResourcePhraseFailed
		}},
		{name: "child owned by another template", change: func(_, child *radappiov1alpha3.DeploymentResource) {
			child.OwnerReferences[0].UID = "other-template"
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			template := &radappiov1alpha3.DeploymentTemplate{ObjectMeta: metav1.ObjectMeta{Name: "template", Namespace: "target", UID: "template-uid"}}
			parent := &radappiov1alpha3.DeploymentResource{
				ObjectMeta: metav1.ObjectMeta{Name: "parent", Namespace: template.Namespace, DeletionTimestamp: new(metav1.Now()),
					Finalizers: []string{DeploymentResourceFinalizer}, OwnerReferences: []metav1.OwnerReference{{
						Kind: deploymentTemplateKind, Name: template.Name, UID: template.UID, Controller: new(true),
					}}},
				Spec: radappiov1alpha3.DeploymentResourceSpec{Id: "/planes/radius/local/resourceGroups/selected/providers/Applications.Core/applications/parent"},
			}
			child := parent.DeepCopy()
			child.Name, child.DeletionTimestamp = "child", nil
			child.Spec.Id = "/planes/radius/local/resourceGroups/selected/providers/Applications.Core/containers/child"
			if tt.change != nil {
				tt.change(parent, child)
			}
			c := fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).WithObjects(template, parent, child).Build()
			r := &DeploymentTemplateReconciler{Client: c}
			cleanup, err := r.pendingDeploymentResourceCleanup(t.Context(), template, []string{child.Spec.Id})
			require.NoError(t, err)
			require.True(t, cleanup.pending)
			require.Equal(t, !tt.allow, cleanup.blocksUpdate)
			require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(parent), parent))
			waiting, err := r.recreateDependencyBlockedOutput(t.Context(), template, parent)
			require.NoError(t, err)
			require.Equal(t, tt.allow, waiting, "only a dependency-blocked, unsubmitted parent DELETE can be retired")
			var got radappiov1alpha3.DeploymentResource
			err = c.Get(t.Context(), k8sclient.ObjectKeyFromObject(parent), &got)
			if tt.allow {
				require.True(t, apierrors.IsNotFound(err))
			} else {
				require.NoError(t, err)
				require.Contains(t, got.Finalizers, DeploymentResourceFinalizer)
			}
		})
	}
}

func Test_DeploymentTemplateReconciler_DependencyOutputConflict(t *testing.T) {
	t.Parallel()
	template := &radappiov1alpha3.DeploymentTemplate{ObjectMeta: metav1.ObjectMeta{Name: "template", Namespace: "target", UID: "template-uid"}}
	parent := &radappiov1alpha3.DeploymentResource{
		ObjectMeta: metav1.ObjectMeta{Name: "parent", Namespace: template.Namespace, UID: "parent-uid", DeletionTimestamp: new(metav1.Now()),
			Finalizers: []string{DeploymentResourceFinalizer}, OwnerReferences: []metav1.OwnerReference{{
				Kind: deploymentTemplateKind, Name: template.Name, UID: template.UID, Controller: new(true),
			}}},
		Spec: radappiov1alpha3.DeploymentResourceSpec{Id: "/planes/radius/local/resourceGroups/selected/providers/Applications.Core/applications/parent"},
	}
	child := parent.DeepCopy()
	child.Name, child.UID, child.DeletionTimestamp = "child", "child-uid", nil
	child.Spec.Id = "/planes/radius/local/resourceGroups/selected/providers/Applications.Core/containers/child"
	c := fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).
		WithStatusSubresource(&radappiov1alpha3.DeploymentResource{}).
		WithObjects(template, parent, child).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c k8sclient.WithWatch, object k8sclient.Object, opts ...k8sclient.UpdateOption) error {
				var started radappiov1alpha3.DeploymentResource
				require.NoError(t, c.Get(ctx, k8sclient.ObjectKeyFromObject(parent), &started))
				started.Status.Phrase = radappiov1alpha3.DeploymentResourcePhraseDeleting
				require.NoError(t, c.Status().Update(ctx, &started))
				return c.Update(ctx, object, opts...)
			},
		}).Build()
	require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(parent), parent))
	r := &DeploymentTemplateReconciler{Client: c}
	recreating, err := r.recreateDependencyBlockedOutput(t.Context(), template, parent)
	require.False(t, recreating)
	require.True(t, apierrors.IsConflict(err), "a concurrent deletion start must prevent retiring its tracker")
	var current radappiov1alpha3.DeploymentResource
	require.NoError(t, c.Get(t.Context(), k8sclient.ObjectKeyFromObject(parent), &current))
	require.Contains(t, current.Finalizers, DeploymentResourceFinalizer)
	require.Equal(t, radappiov1alpha3.DeploymentResourcePhraseDeleting, current.Status.Phrase)
}

func Test_DeploymentTemplateReconciler_DeleteRestoringOwner(t *testing.T) {
	SkipWithoutEnvironment(t)
	for _, parentType := range []string{"applications", "environments"} {
		t.Run(parentType, func(t *testing.T) {
			ctx := t.Context()
			c, err := k8sclient.New(config, k8sclient.Options{Scheme: scheme})
			require.NoError(t, err)
			namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "delete-restoring-" + parentType}}
			require.NoError(t, c.Create(ctx, namespace))
			t.Cleanup(func() {
				cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
				defer cancel()
				require.NoError(t, k8sclient.IgnoreNotFound(c.Delete(cleanupCtx, namespace)))
			})
			provider, err := sdkclients.NewDefaultProviderConfig(namespace.Name).String()
			require.NoError(t, err)
			dt := &radappiov1alpha3.DeploymentTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "template", Namespace: namespace.Name, Finalizers: []string{DeploymentTemplateFinalizer}},
				Spec:       radappiov1alpha3.DeploymentTemplateSpec{Template: "{}", ProviderConfig: provider},
			}
			require.NoError(t, c.Create(ctx, dt))
			scope := "/planes/radius/local/resourceGroups/" + namespace.Name
			parentID := scope + "/providers/Applications.Core/" + parentType + "/parent"
			childID := scope + "/providers/Applications.Core/containers/child"
			owner := metav1.OwnerReference{APIVersion: radappiov1alpha3.GroupVersion.String(), Kind: deploymentTemplateKind,
				Name: dt.Name, UID: dt.UID, Controller: new(true)}
			parent := &radappiov1alpha3.DeploymentResource{
				ObjectMeta: metav1.ObjectMeta{Name: "parent", Namespace: dt.Namespace,
					Finalizers: []string{DeploymentResourceFinalizer}, OwnerReferences: []metav1.OwnerReference{owner}},
				Spec: radappiov1alpha3.DeploymentResourceSpec{Id: parentID},
			}
			child := parent.DeepCopy()
			child.Name, child.Spec.Id = "child", childID
			require.NoError(t, c.Create(ctx, parent))
			require.NoError(t, c.Create(ctx, child))
			require.NoError(t, c.Delete(ctx, parent))
			oldUID := parent.UID
			mock := sdkclients.NewMockResourceDeploymentsClient()
			poller, err := mock.CreateOrUpdate(ctx, sdkclients.Deployment{}, "completed", "test")
			require.NoError(t, err)
			token, err := poller.ResumeToken()
			require.NoError(t, err)
			mock.CompleteOperation(token, func(state *sdkclients.OperationState) {
				state.Value = sdkclients.ClientCreateOrUpdateResponse{Properties: &armdeployments.DeploymentPropertiesExtended{
					OutputResources: []*armdeployments.ResourceReference{{ID: new(parentID)}, {ID: new(childID)}},
				}}
			})
			dt.Status.OutputResources = []string{childID}
			dt.Status.Operation = &radappiov1alpha3.ResourceOperation{OperationKind: radappiov1alpha3.OperationKindPut, ResumeToken: token}
			dt.Status.OperationSpecHash, err = computeHash(dt)
			require.NoError(t, err)
			require.NoError(t, c.Status().Update(ctx, dt))
			policy, err := NewFluxSourcePolicy(hostoptions.FluxOptions{}, c)
			require.NoError(t, err)
			backend := &sourceTestDeployments{ResourceDeploymentsClient: mock}
			r := &DeploymentTemplateReconciler{Client: c, Scheme: scheme, SourcePolicy: policy,
				EventRecorder: record.NewFakeRecorder(10), ResourceDeploymentsClient: backend}
			request := ctrl.Request{NamespacedName: k8sclient.ObjectKeyFromObject(dt)}
			result, err := r.Reconcile(ctx, request)
			require.NoError(t, err)
			require.Positive(t, result.RequeueAfter)
			require.True(t, apierrors.IsNotFound(c.Get(ctx, k8sclient.ObjectKeyFromObject(parent), &radappiov1alpha3.DeploymentResource{})))
			require.NoError(t, c.Get(ctx, request.NamespacedName, dt))
			require.Equal(t, token, dt.Status.Operation.ResumeToken)
			require.NoError(t, c.Delete(ctx, dt))

			_, err = r.Reconcile(ctx, request)
			require.ErrorContains(t, err, "owned resources")
			require.NoError(t, c.Get(ctx, k8sclient.ObjectKeyFromObject(parent), parent))
			require.NotEqual(t, oldUID, parent.UID)
			require.NotNil(t, parent.DeletionTimestamp)
			require.Contains(t, parent.Finalizers, DeploymentResourceFinalizer)
			resources := &DeploymentResourceReconciler{Client: c, SourcePolicy: policy,
				EventRecorder: record.NewFakeRecorder(10), ResourceDeploymentsClient: backend}
			_, err = resources.Reconcile(ctx, ctrl.Request{NamespacedName: k8sclient.ObjectKeyFromObject(parent)})
			require.NoError(t, err)
			require.Zero(t, backend.deletes, "restored parent cleanup must still wait for the child")
			for _, output := range []*radappiov1alpha3.DeploymentResource{child, parent} {
				key := k8sclient.ObjectKeyFromObject(output)
				_, err = resources.Reconcile(ctx, ctrl.Request{NamespacedName: key})
				require.NoError(t, err)
				require.NoError(t, c.Get(ctx, key, output))
				require.NotNil(t, output.Status.Operation)
				mock.CompleteOperation(output.Status.Operation.ResumeToken, nil)
				_, err = resources.Reconcile(ctx, ctrl.Request{NamespacedName: key})
				require.NoError(t, err)
				require.True(t, apierrors.IsNotFound(c.Get(ctx, key, &radappiov1alpha3.DeploymentResource{})))
			}
			_, err = r.Reconcile(ctx, request)
			require.NoError(t, err)
			require.True(t, apierrors.IsNotFound(c.Get(ctx, request.NamespacedName, &radappiov1alpha3.DeploymentTemplate{})))
			require.ElementsMatch(t, []string{parentID, childID}, mock.DeletedResourceIDs())
			require.Zero(t, backend.creates)
		})
	}
}

const (
	deploymentTemplateTestWaitDuration            = time.Second * 10
	deploymentTemplateTestWaitInterval            = time.Millisecond * 200
	deploymentTemplateTestControllerDelayInterval = time.Millisecond * 100
)

func SetupDeploymentTemplateTest(t *testing.T) (*mockRadiusClient, *sdkclients.MockResourceDeploymentsClient, k8sclient.Client) {
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
	sourcePolicy, err := NewFluxSourcePolicy(hostoptions.FluxOptions{}, mgr.GetAPIReader())
	require.NoError(t, err)

	// Set up DeploymentTemplateReconciler.
	//nolint:staticcheck // SA1019: GetEventRecorderFor is deprecated but migration to new events API requires significant refactoring
	err = (&DeploymentTemplateReconciler{
		Client:                    mgr.GetClient(),
		Scheme:                    mgr.GetScheme(),
		EventRecorder:             mgr.GetEventRecorderFor("deploymenttemplate-controller"),
		Radius:                    mockRadiusClient,
		ResourceDeploymentsClient: mockResourceDeploymentsClient,
		DelayInterval:             deploymentTemplateTestControllerDelayInterval,
		SourcePolicy:              sourcePolicy,
	}).SetupWithManager(mgr)
	require.NoError(t, err)

	// Set up DeploymentResourceReconciler.
	//nolint:staticcheck // SA1019: GetEventRecorderFor is deprecated but migration to new events API requires significant refactoring
	err = (&DeploymentResourceReconciler{
		Client:                    mgr.GetClient(),
		Scheme:                    mgr.GetScheme(),
		EventRecorder:             mgr.GetEventRecorderFor("deploymentresource-controller"),
		Radius:                    mockRadiusClient,
		ResourceDeploymentsClient: mockResourceDeploymentsClient,
		DelayInterval:             DeploymentResourceTestControllerDelayInterval,
		DeleteRetryInterval:       DeploymentResourceTestControllerDelayInterval,
		SourcePolicy:              sourcePolicy,
	}).SetupWithManager(mgr)
	require.NoError(t, err)

	startManager(t, mgr, ctx, cancel)

	return mockRadiusClient, mockResourceDeploymentsClient, mgr.GetClient()
}

func Test_DeploymentTemplateReconciler_ComputeHash(t *testing.T) {
	testcases := []struct {
		name               string
		deploymentTemplate *radappiov1alpha3.DeploymentTemplate
		expected           string
	}{
		{
			name: "empty",
			deploymentTemplate: &radappiov1alpha3.DeploymentTemplate{
				Spec: radappiov1alpha3.DeploymentTemplateSpec{},
			},
			expected: "44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
		},
		{
			name: "simple",
			deploymentTemplate: &radappiov1alpha3.DeploymentTemplate{
				Spec: radappiov1alpha3.DeploymentTemplateSpec{
					Template:       "{}",
					Parameters:     map[string]string{},
					ProviderConfig: "{}",
				},
			},
			expected: "950829dab3e33b280e2ef52a7484c51d19a51498a74aab1d7f4d2781d6e65953",
		},
		{
			name: "complex",
			deploymentTemplate: &radappiov1alpha3.DeploymentTemplate{
				Spec: radappiov1alpha3.DeploymentTemplateSpec{
					Template:       `{"resources":[{"type":"Microsoft.Resources/deployments","apiVersion":"2020-06-01","name":"test-deploymenttemplate-basic","properties":{"mode":"Incremental","template":{},"parameters":{}}}]}`,
					Parameters:     map[string]string{"param1": "value1", "param2": "value2"},
					ProviderConfig: `{"AWS":{"type":"aws","value":{"scope":"scope"}}}`,
				},
			},
			expected: "ff16a7bcb2db8d7c1280eb761b376939dfc3856a74cfd77b986a1c0f25abf5dd",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			hash, err := computeHash(tc.deploymentTemplate)
			require.NoError(t, err)
			require.Equal(t, tc.expected, hash)
		})
	}
}

func Test_DeploymentTemplateReconciler_IsUpToDate(t *testing.T) {
	testcases := []struct {
		name               string
		deploymentTemplate *radappiov1alpha3.DeploymentTemplate
		expected           bool
	}{
		{
			name: "up-to-date",
			deploymentTemplate: &radappiov1alpha3.DeploymentTemplate{
				Spec: radappiov1alpha3.DeploymentTemplateSpec{
					Template:       "{}",
					Parameters:     map[string]string{},
					ProviderConfig: "{}",
				},
				Status: radappiov1alpha3.DeploymentTemplateStatus{
					StatusHash: "950829dab3e33b280e2ef52a7484c51d19a51498a74aab1d7f4d2781d6e65953",
				},
			},
			expected: true,
		},
		{
			// Verifies the SHA-1 -> SHA-256 migration is non-breaking: a StatusHash written
			// by an older version of Radius (legacy SHA-1 of the "simple" spec) is still
			// considered up-to-date, so upgrading does not trigger an unnecessary deployment.
			name: "legacy-hash-up-to-date",
			deploymentTemplate: &radappiov1alpha3.DeploymentTemplate{
				Spec: radappiov1alpha3.DeploymentTemplateSpec{
					Template:       "{}",
					Parameters:     map[string]string{},
					ProviderConfig: "{}",
				},
				Status: radappiov1alpha3.DeploymentTemplateStatus{
					StatusHash: "47ee899e74561942ee36a02ffd80be955e251583",
				},
			},
			expected: true,
		},
		{
			name: "not-up-to-date",
			deploymentTemplate: &radappiov1alpha3.DeploymentTemplate{
				Spec: radappiov1alpha3.DeploymentTemplateSpec{
					Template:       "{}",
					Parameters:     map[string]string{},
					ProviderConfig: "{}",
				},
				Status: radappiov1alpha3.DeploymentTemplateStatus{
					StatusHash: "incorrecthash",
				},
			},
			expected: false,
		},
		{
			name: "complex-up-to-date",
			deploymentTemplate: &radappiov1alpha3.DeploymentTemplate{
				Spec: radappiov1alpha3.DeploymentTemplateSpec{
					Template:       `{"resources":[{"type":"Microsoft.Resources/deployments","apiVersion":"2020-06-01","name":"test-deploymenttemplate-basic","properties":{"mode":"Incremental","template":{},"parameters":{}}}]}`,
					Parameters:     map[string]string{"param1": "value1", "param2": "value2"},
					ProviderConfig: `{"AWS":{"type":"aws","value":{"scope":"scope"}}}`,
				},
				Status: radappiov1alpha3.DeploymentTemplateStatus{
					StatusHash: "ff16a7bcb2db8d7c1280eb761b376939dfc3856a74cfd77b986a1c0f25abf5dd",
				},
			},
			expected: true,
		},
		{
			name: "complex-not-up-to-date",
			deploymentTemplate: &radappiov1alpha3.DeploymentTemplate{
				Spec: radappiov1alpha3.DeploymentTemplateSpec{
					Template:       `{"resources":[{"type":"Microsoft.Resources/deployments","apiVersion":"2020-06-01","name":"test-deploymenttemplate-basic","properties":{"mode":"Incremental","template":{},"parameters":{}}}]}`,
					Parameters:     map[string]string{"param1": "value1", "param2": "value2"},
					ProviderConfig: `{"AWS":{"type":"aws","value":{"scope":"scope"}}}`,
				},
				Status: radappiov1alpha3.DeploymentTemplateStatus{
					StatusHash: "incorrecthash",
				},
			},
			expected: false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			isUpToDate := isUpToDate(tc.deploymentTemplate)
			require.Equal(t, tc.expected, isUpToDate)
		})
	}
}

func Test_ParseDeploymentScopeFromProviderConfig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		providerConfig string
		wantScope      string
		wantErr        bool
	}{
		{
			name:           "valid: provider with scope",
			providerConfig: `{"deployments":{"type":"deployments","value":{"scope":"deploymentsscope"}}}`,
			wantScope:      "deploymentsscope",
			wantErr:        false,
		},
		{
			name:           "invalid: deployments scope not present",
			providerConfig: `{"radius":{"type":"radius","value":{"scope":"deploymentsscope"}}}`,
			wantErr:        true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			scope, err := ParseDeploymentScopeFromProviderConfig(tc.providerConfig)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantScope, scope)
		})
	}
}

func Test_DeploymentTemplateReconciler_Basic(t *testing.T) {
	// This test tests the basic functionality of the DeploymentTemplate controller.
	// It creates a DeploymentTemplate (with an empty template field),
	// waits for it to be ready, and then deletes it.
	//
	// This is the same structure as all of the following tests.

	ctx := t.Context()

	// Set up the test.
	_, mockDeploymentClient, k8sClient := SetupDeploymentTemplateTest(t)
	testNamespace := "deploymenttemplate-basic"
	testName := "test-deploymenttemplate-basic"
	template := "{}"
	parameters := map[string]string{}
	providerConfig, err := sdkclients.NewDefaultProviderConfig(testNamespace).String()
	require.NoError(t, err)

	// Create k8s namespace for the test.
	namespacedName := types.NamespacedName{Namespace: testNamespace, Name: testName}
	err = k8sClient.Create(ctx, &corev1.Namespace{Name: testNamespace})
	require.NoError(t, err)

	// Create the DeploymentTemplate resource.
	deploymentTemplate := makeDeploymentTemplate(namespacedName, template, providerConfig, parameters)
	err = k8sClient.Create(ctx, deploymentTemplate)
	require.NoError(t, err)

	// Wait for the DeploymentTemplate to enter the Updating state.
	status := waitForDeploymentTemplateStateUpdating(t, k8sClient, namespacedName, nil)

	// Complete the operation.
	mockDeploymentClient.CompleteOperation(status.Operation.ResumeToken, nil)

	// DeploymentTemplate should be Ready after the operation completes.
	status = waitForDeploymentTemplateStateReady(t, k8sClient, namespacedName)

	// Verify that the DeploymentTemplate desired state contains the expected properties.
	expectedDeploymentTemplateSpec := &radappiov1alpha3.DeploymentTemplate{
		Spec: radappiov1alpha3.DeploymentTemplateSpec{
			Template:       template,
			Parameters:     parameters,
			ProviderConfig: providerConfig,
		},
	}
	expectedStatusHash, err := computeHash(expectedDeploymentTemplateSpec)
	require.NoError(t, err)
	require.Equal(t, expectedStatusHash, status.StatusHash)

	// Trigger deletion of the DeploymentTemplate.
	err = k8sClient.Delete(ctx, deploymentTemplate)
	require.NoError(t, err)

	// Wait for the DeploymentTemplate to be deleted.
	waitForDeploymentTemplateStateDeleted(t, k8sClient, namespacedName)
}

func Test_DeploymentTemplateReconciler_FailureRecovery(t *testing.T) {
	// This test tests our ability to recover from failed operations inside Radius.
	//
	// We use the mock client to simulate the failure of update and delete operations
	// and verify that the controller will (eventually) retry these operations.

	ctx := t.Context()

	// Set up the test.
	_, mockDeploymentClient, k8sClient := SetupDeploymentTemplateTest(t)
	testNamespace := "deploymenttemplate-failurerecovery"
	testName := "test-deploymenttemplate-failurerecovery"
	template := "{}"
	parameters := map[string]string{}
	providerConfig, err := sdkclients.NewDefaultProviderConfig(testNamespace).String()
	require.NoError(t, err)

	// Create k8s namespace for the test.
	namespacedName := types.NamespacedName{Namespace: testNamespace, Name: testName}
	err = k8sClient.Create(ctx, &corev1.Namespace{Name: testNamespace})
	require.NoError(t, err)

	// Create the DeploymentTemplate resource.
	deploymentTemplate := makeDeploymentTemplate(namespacedName, template, providerConfig, parameters)
	err = k8sClient.Create(ctx, deploymentTemplate)
	require.NoError(t, err)

	// Wait for the DeploymentTemplate to enter the Updating state.
	status := waitForDeploymentTemplateStateUpdating(t, k8sClient, namespacedName, nil)

	// Complete the operation, but make it fail.
	operation := status.Operation
	mockDeploymentClient.CompleteOperation(status.Operation.ResumeToken, func(state *sdkclients.OperationState) {
		state.Err = errors.New("failure")

		resource, ok := mockDeploymentClient.GetResource(state.ResourceID)
		require.True(t, ok, "failed to find resource")

		resource.Properties.ProvisioningState = to.Ptr(armdeployments.ProvisioningStateFailed)
		state.Value = sdkclients.ClientCreateOrUpdateResponse{Properties: resource.Properties}
	})

	// DeploymentTemplate should (eventually) start a new provisioning operation
	status = waitForDeploymentTemplateStateUpdating(t, k8sClient, namespacedName, operation)

	// Complete the operation, successfully this time.
	mockDeploymentClient.CompleteOperation(status.Operation.ResumeToken, nil)
	status = waitForDeploymentTemplateStateReady(t, k8sClient, namespacedName)

	// Verify that the DeploymentTemplate desired state contains the expected properties.
	expectedDeploymentTemplateSpec := &radappiov1alpha3.DeploymentTemplate{
		Spec: radappiov1alpha3.DeploymentTemplateSpec{
			Template:       template,
			Parameters:     parameters,
			ProviderConfig: providerConfig,
		},
	}
	expectedStatusHash, err := computeHash(expectedDeploymentTemplateSpec)
	require.NoError(t, err)
	require.Equal(t, expectedStatusHash, status.StatusHash)

	// Trigger deletion of the DeploymentTemplate.
	err = k8sClient.Delete(ctx, deploymentTemplate)
	require.NoError(t, err)

	// Wait for the DeploymentTemplate to be deleted.
	waitForDeploymentTemplateStateDeleted(t, k8sClient, namespacedName)
}

func Test_DeploymentTemplateReconciler_SpecChangedDuringOperation(t *testing.T) {
	ctx := t.Context()
	_, mockDeploymentClient, k8sClient := SetupDeploymentTemplateTest(t)
	namespacedName := types.NamespacedName{Namespace: "deploymenttemplate-spec-change", Name: "test-deploymenttemplate"}
	providerConfig, err := sdkclients.NewDefaultProviderConfig(namespacedName.Namespace).String()
	require.NoError(t, err)
	require.NoError(t, k8sClient.Create(ctx, &corev1.Namespace{Name: namespacedName.Namespace}))

	deploymentTemplate := makeDeploymentTemplate(namespacedName, "{}", providerConfig, nil)
	require.NoError(t, k8sClient.Create(ctx, deploymentTemplate))
	status := waitForDeploymentTemplateStateUpdating(t, k8sClient, namespacedName, nil)

	// Change the spec while the first operation runs.
	require.NoError(t, k8sClient.Get(ctx, namespacedName, deploymentTemplate))
	deploymentTemplate.Spec.Template = `{"resources":[]}`
	require.NoError(t, k8sClient.Update(ctx, deploymentTemplate))
	updatedHash, err := computeHash(deploymentTemplate)
	require.NoError(t, err)

	// The first operation's success must start a second one for the changed spec.
	mockDeploymentClient.CompleteOperation(status.Operation.ResumeToken, nil)
	status = waitForDeploymentTemplateStateUpdating(t, k8sClient, namespacedName, status.Operation)
	mockDeploymentClient.CompleteOperation(status.Operation.ResumeToken, nil)
	status = waitForDeploymentTemplateStateReady(t, k8sClient, namespacedName)
	require.Equal(t, updatedHash, status.StatusHash)

	require.NoError(t, k8sClient.Delete(ctx, deploymentTemplate))
	waitForDeploymentTemplateStateDeleted(t, k8sClient, namespacedName)
}

func Test_DeploymentTemplateReconciler_WithResources(t *testing.T) {
	// This test tests the ability to handle deployments of
	// resources created by the DeploymentTemplate.

	ctx := t.Context()

	// Set up the test.
	_, mockDeploymentClient, k8sClient := SetupDeploymentTemplateTest(t)
	testNamespace := "deploymenttemplate-withresources"
	testName := "test-deploymenttemplate-withresources"
	template := readFileIntoTemplate(t, "deploymenttemplate-withresources.json")
	parameters := map[string]string{}
	providerConfig, err := sdkclients.NewDefaultProviderConfig(testNamespace).String()
	require.NoError(t, err)

	// Create k8s namespace for the test.
	namespacedName := types.NamespacedName{Namespace: testNamespace, Name: testName}
	err = k8sClient.Create(ctx, &corev1.Namespace{Name: testNamespace})
	require.NoError(t, err)

	// Create the DeploymentTemplate resource.
	deploymentTemplate := makeDeploymentTemplate(namespacedName, template, providerConfig, parameters)
	err = k8sClient.Create(ctx, deploymentTemplate)
	require.NoError(t, err)

	// Wait for the DeploymentTemplate to enter the Updating state.
	status := waitForDeploymentTemplateStateUpdating(t, k8sClient, namespacedName, nil)

	// Complete the operation.
	mockDeploymentClient.CompleteOperation(status.Operation.ResumeToken, func(state *sdkclients.OperationState) {
		resource, ok := mockDeploymentClient.GetResource(state.ResourceID)
		require.True(t, ok, "failed to find resource")

		resource.Properties.OutputResources = []*armdeployments.ResourceReference{
			{ID: new("/planes/radius/local/resourceGroups/deploymenttemplate-withresources/providers/Applications.Core/environments/deploymenttemplate-withresources-env")},
		}
		state.Value = sdkclients.ClientCreateOrUpdateResponse{Properties: resource.Properties}
	})

	// DeploymentTemplate should be ready after the operation completes.
	status = waitForDeploymentTemplateStateReady(t, k8sClient, namespacedName)

	// The dependencies (DeploymentResource resources) should be created.
	dependencyName := types.NamespacedName{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-withresources-env"}
	dependencyStatus := waitForDeploymentResourceStateReady(t, k8sClient, dependencyName)
	require.Equal(t, "/planes/radius/local/resourceGroups/deploymenttemplate-withresources/providers/Applications.Core/environments/deploymenttemplate-withresources-env", dependencyStatus.Id)

	// Verify that the DeploymentTemplate desired state contains the expected properties.
	expectedDeploymentTemplateSpec := &radappiov1alpha3.DeploymentTemplate{
		Spec: radappiov1alpha3.DeploymentTemplateSpec{
			Template:       template,
			Parameters:     parameters,
			ProviderConfig: providerConfig,
		},
	}
	expectedStatusHash, err := computeHash(expectedDeploymentTemplateSpec)
	require.NoError(t, err)
	require.Equal(t, expectedStatusHash, status.StatusHash)

	// Trigger deletion of the DeploymentTemplate.
	err = k8sClient.Delete(ctx, deploymentTemplate)
	require.NoError(t, err)

	// The DeploymentTemplate should be in the deleting state.
	waitForDeploymentTemplateStateDeleting(t, k8sClient, namespacedName)

	// Get the status of the dependency (DeploymentResource resource).
	dependencyStatus = waitForDeploymentResourceStateDeleting(t, k8sClient, dependencyName, nil)

	// Complete the delete operation on the DeploymentResource.
	mockDeploymentClient.CompleteOperation(dependencyStatus.Operation.ResumeToken, nil)

	// Wait for the DeploymentTemplate to be deleted.
	waitForDeploymentResourceDeleted(t, k8sClient, dependencyName)
	waitForDeploymentTemplateStateDeleted(t, k8sClient, namespacedName)
}

func Test_DeploymentTemplateReconciler_Update(t *testing.T) {
	// This test tests our ability to update a DeploymentTemplate.
	// We create a DeploymentTemplate, update it, and verify that the Radius resource is updated accordingly.

	ctx := t.Context()

	// Set up the test.
	_, mockDeploymentClient, k8sClient := SetupDeploymentTemplateTest(t)
	testNamespace := "deploymenttemplate-update"
	testName := "test-deploymenttemplate-update"
	template := readFileIntoTemplate(t, "deploymenttemplate-update-1.json")
	parameters := map[string]string{}
	providerConfig, err := sdkclients.NewDefaultProviderConfig(testNamespace).String()
	require.NoError(t, err)

	// Create k8s namespace for the test.
	namespacedName := types.NamespacedName{Namespace: testNamespace, Name: testName}
	err = k8sClient.Create(ctx, &corev1.Namespace{Name: testNamespace})
	require.NoError(t, err)

	// Create the DeploymentTemplate resource.
	deploymentTemplate := makeDeploymentTemplate(namespacedName, template, providerConfig, parameters)
	err = k8sClient.Create(ctx, deploymentTemplate)
	require.NoError(t, err)

	// Wait for the DeploymentTemplate to enter the Updating state.
	status := waitForDeploymentTemplateStateUpdating(t, k8sClient, namespacedName, nil)

	// Complete the operation.
	mockDeploymentClient.CompleteOperation(status.Operation.ResumeToken, func(state *sdkclients.OperationState) {
		resource, ok := mockDeploymentClient.GetResource(state.ResourceID)
		require.True(t, ok, "failed to find resource")

		resource.Properties.OutputResources = []*armdeployments.ResourceReference{
			{ID: new("/planes/radius/local/resourceGroups/deploymenttemplate-update/providers/Applications.Core/environments/deploymenttemplate-update-env")},
		}
		state.Value = sdkclients.ClientCreateOrUpdateResponse{Properties: resource.Properties}
	})

	// DeploymentTemplate should be ready after the operation completes.
	status = waitForDeploymentTemplateStateReady(t, k8sClient, namespacedName)

	// The dependencies (DeploymentResource resources) should be created.
	dependencyName := types.NamespacedName{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-update-env"}
	dependencyStatus := waitForDeploymentResourceStateReady(t, k8sClient, dependencyName)
	require.Equal(t, "/planes/radius/local/resourceGroups/deploymenttemplate-update/providers/Applications.Core/environments/deploymenttemplate-update-env", dependencyStatus.Id)

	// Verify that the DeploymentTemplate desired state contains the expected properties.
	expectedDeploymentTemplateSpec := &radappiov1alpha3.DeploymentTemplate{
		Spec: radappiov1alpha3.DeploymentTemplateSpec{
			Template:       template,
			Parameters:     parameters,
			ProviderConfig: providerConfig,
		},
	}
	expectedStatusHash, err := computeHash(expectedDeploymentTemplateSpec)
	require.NoError(t, err)
	require.Equal(t, expectedStatusHash, status.StatusHash)

	// Now, we will re-deploy the DeploymentTemplate with a new template.

	// Get the DeploymentTemplate resource.
	newDeploymentTemplate := radappiov1alpha3.DeploymentTemplate{}
	err = k8sClient.Get(ctx, namespacedName, &newDeploymentTemplate)
	require.NoError(t, err)

	// Update the template field on the DeploymentTemplate.
	template = readFileIntoTemplate(t, "deploymenttemplate-update-2.json")
	newDeploymentTemplate.Spec.Template = string(template)

	// Update the DeploymentTemplate resource.
	err = k8sClient.Update(ctx, &newDeploymentTemplate)
	require.NoError(t, err)

	// Now, the DeploymentTemplate should re-enter the Updating state.
	status = waitForDeploymentTemplateStateUpdating(t, k8sClient, namespacedName, nil)

	// Complete the operation again.
	mockDeploymentClient.CompleteOperation(status.Operation.ResumeToken, func(state *sdkclients.OperationState) {
		resource, ok := mockDeploymentClient.GetResource(state.ResourceID)
		require.True(t, ok, "failed to find resource")

		resource.Properties.OutputResources = []*armdeployments.ResourceReference{
			{ID: new("/planes/radius/local/resourceGroups/deploymenttemplate-update/providers/Applications.Core/environments/deploymenttemplate-update-env")},
		}
		state.Value = sdkclients.ClientCreateOrUpdateResponse{Properties: resource.Properties}
	})

	// DeploymentTemplate should be Ready again after the operation completes.
	status = waitForDeploymentTemplateStateReady(t, k8sClient, namespacedName)

	// The dependencies (DeploymentResource resources) should also be Ready.
	dependencyName = types.NamespacedName{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-update-env"}
	dependencyStatus = waitForDeploymentResourceStateReady(t, k8sClient, dependencyName)
	require.Equal(t, "/planes/radius/local/resourceGroups/deploymenttemplate-update/providers/Applications.Core/environments/deploymenttemplate-update-env", dependencyStatus.Id)

	// Verify that the DeploymentTemplate contains the expected properties.
	expectedDeploymentTemplateSpec = &radappiov1alpha3.DeploymentTemplate{
		Spec: radappiov1alpha3.DeploymentTemplateSpec{
			Template:       template,
			Parameters:     parameters,
			ProviderConfig: providerConfig,
		},
	}
	expectedStatusHash, err = computeHash(expectedDeploymentTemplateSpec)
	require.NoError(t, err)
	require.Equal(t, expectedStatusHash, status.StatusHash)

	// Trigger deletion of the DeploymentTemplate.
	err = k8sClient.Delete(ctx, deploymentTemplate)
	require.NoError(t, err)

	// The DeploymentTemplate should be in the deleting state.
	waitForDeploymentTemplateStateDeleting(t, k8sClient, namespacedName)

	// Get the status of the dependency (DeploymentResource resource).
	dependencyStatus = waitForDeploymentResourceStateDeleting(t, k8sClient, dependencyName, nil)

	// Complete the delete operation on the DeploymentResource.
	mockDeploymentClient.CompleteOperation(dependencyStatus.Operation.ResumeToken, nil)

	// Wait for the DeploymentTemplate to be deleted.
	waitForDeploymentResourceDeleted(t, k8sClient, dependencyName)
	waitForDeploymentTemplateStateDeleted(t, k8sClient, namespacedName)
}

func Test_DeploymentTemplateReconciler_OutputResources(t *testing.T) {
	// This test tests the ability to perform diff detection on
	// the OutputResources field of the DeploymentTemplate.
	// We create a DeploymentTemplate with some resources,
	// update the DeploymentTemplate to remove some resources,
	// and verify that the diff is correct.

	ctx := t.Context()

	// Set up the test.
	_, mockDeploymentClient, k8sClient := SetupDeploymentTemplateTest(t)
	testNamespace := "deploymenttemplate-outputresources"
	testName := "test-deploymenttemplate-outputresources"
	template := readFileIntoTemplate(t, "deploymenttemplate-outputresources-1.json")
	parameters := map[string]string{}
	providerConfig, err := sdkclients.NewDefaultProviderConfig(testNamespace).String()
	require.NoError(t, err)

	// Create k8s namespace for the test.
	namespacedName := types.NamespacedName{Namespace: testNamespace, Name: testName}
	err = k8sClient.Create(ctx, &corev1.Namespace{Name: testNamespace})
	require.NoError(t, err)

	// Create the DeploymentTemplate resource.
	deploymentTemplate := makeDeploymentTemplate(namespacedName, template, providerConfig, parameters)
	err = k8sClient.Create(ctx, deploymentTemplate)
	require.NoError(t, err)

	// Wait for the DeploymentTemplate to enter the Updating state.
	status := waitForDeploymentTemplateStateUpdating(t, k8sClient, namespacedName, nil)

	// Complete the operation.
	mockDeploymentClient.CompleteOperation(status.Operation.ResumeToken, func(state *sdkclients.OperationState) {
		resource, ok := mockDeploymentClient.GetResource(state.ResourceID)
		require.True(t, ok, "failed to find resource")

		resource.Properties.OutputResources = []*armdeployments.ResourceReference{
			{ID: new("/planes/radius/local/resourceGroups/deploymenttemplate-outputresources/providers/Applications.Core/environments/deploymenttemplate-outputresources-environment")},
			{ID: new("/planes/radius/local/resourceGroups/deploymenttemplate-outputresources/providers/Applications.Core/applications/deploymenttemplate-outputresources-application")},
			{ID: new("/planes/radius/local/resourceGroups/deploymenttemplate-outputresources/providers/Applications.Core/containers/deploymenttemplate-outputresources-container")},
		}
		state.Value = sdkclients.ClientCreateOrUpdateResponse{Properties: resource.Properties}
	})

	// DeploymentTemplate should be ready after the operation completes.
	status = waitForDeploymentTemplateStateReady(t, k8sClient, namespacedName)

	// The dependencies (DeploymentResource resources) should be created.
	dependencies := []struct {
		resourceID     string
		namespacedName types.NamespacedName
	}{
		{
			resourceID:     "/planes/radius/local/resourceGroups/deploymenttemplate-outputresources/providers/Applications.Core/environments/deploymenttemplate-outputresources-environment",
			namespacedName: types.NamespacedName{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-outputresources-environment"},
		},
		{
			resourceID:     "/planes/radius/local/resourceGroups/deploymenttemplate-outputresources/providers/Applications.Core/applications/deploymenttemplate-outputresources-application",
			namespacedName: types.NamespacedName{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-outputresources-application"},
		},
		{
			resourceID:     "/planes/radius/local/resourceGroups/deploymenttemplate-outputresources/providers/Applications.Core/containers/deploymenttemplate-outputresources-container",
			namespacedName: types.NamespacedName{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-outputresources-container"},
		},
	}
	for _, dependency := range dependencies {
		dependencyStatus := waitForDeploymentResourceStateReady(t, k8sClient, dependency.namespacedName)
		require.Equal(t, dependency.resourceID, dependencyStatus.Id)
	}

	// Verify that the DeploymentTemplate desired state contains the expected properties.
	expectedDeploymentTemplateSpec := &radappiov1alpha3.DeploymentTemplate{
		Spec: radappiov1alpha3.DeploymentTemplateSpec{
			Template:       template,
			Parameters:     parameters,
			ProviderConfig: providerConfig,
		},
	}
	expectedStatusHash, err := computeHash(expectedDeploymentTemplateSpec)
	require.NoError(t, err)
	require.Equal(t, expectedStatusHash, status.StatusHash)

	// Now, we will re-deploy the DeploymentTemplate with a new template.
	// This template has the container resource removed.

	// Get the DeploymentTemplate resource.
	newDeploymentTemplate := radappiov1alpha3.DeploymentTemplate{}
	err = k8sClient.Get(ctx, namespacedName, &newDeploymentTemplate)
	require.NoError(t, err)

	// Update the template field on the DeploymentTemplate.
	template = readFileIntoTemplate(t, "deploymenttemplate-outputresources-2.json")
	newDeploymentTemplate.Spec.Template = string(template)

	// Update the DeploymentTemplate resource.
	err = k8sClient.Update(ctx, &newDeploymentTemplate)
	require.NoError(t, err)

	// Now, the DeploymentTemplate should re-enter the Updating state.
	status = waitForDeploymentTemplateStateUpdating(t, k8sClient, namespacedName, nil)

	// Complete the operation again, with a different set of output resources.
	mockDeploymentClient.CompleteOperation(status.Operation.ResumeToken, func(state *sdkclients.OperationState) {
		resource, ok := mockDeploymentClient.GetResource(state.ResourceID)
		require.True(t, ok, "failed to find resource")

		resource.Properties.OutputResources = []*armdeployments.ResourceReference{
			{ID: new("/planes/radius/local/resourceGroups/deploymenttemplate-outputresources/providers/Applications.Core/environments/deploymenttemplate-outputresources-environment")},
			{ID: new("/planes/radius/local/resourceGroups/deploymenttemplate-outputresources/providers/Applications.Core/applications/deploymenttemplate-outputresources-application")},
		}
		state.Value = sdkclients.ClientCreateOrUpdateResponse{Properties: resource.Properties}
	})

	// Complete the delete operation on the container resource.
	dependencyName := types.NamespacedName{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-outputresources-container"}
	dependencyStatus := waitForDeploymentResourceStateDeleting(t, k8sClient, dependencyName, nil)
	mockDeploymentClient.CompleteOperation(dependencyStatus.Operation.ResumeToken, nil)

	// DeploymentTemplate should be Ready again after the operation completes.
	status = waitForDeploymentTemplateStateReady(t, k8sClient, namespacedName)

	// The dependencies (DeploymentResource resources) should be in the Ready state.
	dependencies = []struct {
		resourceID     string
		namespacedName types.NamespacedName
	}{
		{
			resourceID:     "/planes/radius/local/resourceGroups/deploymenttemplate-outputresources/providers/Applications.Core/environments/deploymenttemplate-outputresources-environment",
			namespacedName: types.NamespacedName{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-outputresources-environment"},
		},
		{
			resourceID:     "/planes/radius/local/resourceGroups/deploymenttemplate-outputresources/providers/Applications.Core/applications/deploymenttemplate-outputresources-application",
			namespacedName: types.NamespacedName{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-outputresources-application"},
		},
	}
	for _, dependency := range dependencies {
		dependencyStatus := waitForDeploymentResourceStateReady(t, k8sClient, dependency.namespacedName)
		require.Equal(t, dependency.resourceID, dependencyStatus.Id)
	}

	// Assert that the container resource has been deleted.
	dependencyName = types.NamespacedName{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-outputresources-container"}
	waitForDeploymentResourceDeleted(t, k8sClient, dependencyName)

	// Check the cluster for the container resource, it should not exist.
	err = k8sClient.Get(ctx, dependencyName, &radappiov1alpha3.DeploymentResource{})
	require.True(t, apierrors.IsNotFound(err), "expected DeploymentResource to be deleted")

	// Verify that the DeploymentTemplate contains the expected properties.
	expectedDeploymentTemplateSpec = &radappiov1alpha3.DeploymentTemplate{
		Spec: radappiov1alpha3.DeploymentTemplateSpec{
			Template:       template,
			Parameters:     map[string]string{},
			ProviderConfig: providerConfig,
		},
	}
	expectedStatusHash, err = computeHash(expectedDeploymentTemplateSpec)
	require.NoError(t, err)
	require.Equal(t, expectedStatusHash, status.StatusHash)

	// Trigger deletion of the DeploymentTemplate.
	err = k8sClient.Delete(ctx, deploymentTemplate)
	require.NoError(t, err)

	// The DeploymentTemplate should be in the deleting state.
	waitForDeploymentTemplateStateDeleting(t, k8sClient, namespacedName)

	// Wait for all of the dependencies (DeploymentResource resources) to be deleted.
	for _, dependency := range dependencies {
		dependencyStatus := waitForDeploymentResourceStateDeleting(t, k8sClient, dependency.namespacedName, nil)
		mockDeploymentClient.CompleteOperation(dependencyStatus.Operation.ResumeToken, nil)
		waitForDeploymentResourceDeleted(t, k8sClient, dependency.namespacedName)
	}

	// Wait for the DeploymentTemplate to be deleted.
	waitForDeploymentTemplateStateDeleted(t, k8sClient, namespacedName)
}

func Test_DeploymentTemplateReconciler_ReaddDuringCleanup(t *testing.T) {
	_, backend, c := SetupDeploymentTemplateTest(t)
	ctx := t.Context()
	namespace := "deploymenttemplate-readd"
	key := types.NamespacedName{Namespace: namespace, Name: "application"}
	require.NoError(t, c.Create(ctx, &corev1.Namespace{Name: namespace}))
	provider, err := sdkclients.NewDefaultProviderConfig(namespace).String()
	require.NoError(t, err)
	template := makeDeploymentTemplate(key, `{"resources":[]}`, provider, map[string]string{"revision": "initial"})
	require.NoError(t, c.Create(ctx, template))
	outputID := "/planes/radius/local/resourceGroups/" + namespace + "/providers/Applications.Core/containers/output"
	outputKey := types.NamespacedName{Namespace: namespace, Name: "output"}
	completePut := func(state *sdkclients.OperationState) {
		state.Value = sdkclients.ClientCreateOrUpdateResponse{Properties: &armdeployments.DeploymentPropertiesExtended{
			OutputResources: []*armdeployments.ResourceReference{{ID: new(outputID)}},
		}}
	}
	status := waitForDeploymentTemplateStateUpdating(t, c, key, nil)
	backend.CompleteOperation(status.Operation.ResumeToken, completePut)
	waitForDeploymentTemplateStateReady(t, c, key)
	waitForDeploymentResourceStateReady(t, c, outputKey)
	var original radappiov1alpha3.DeploymentResource
	require.NoError(t, c.Get(ctx, outputKey, &original))

	require.NoError(t, c.Get(ctx, key, template))
	template.Spec.Parameters["revision"] = "removed"
	require.NoError(t, c.Update(ctx, template))
	status = waitForDeploymentTemplateStateUpdating(t, c, key, nil)
	backend.CompleteOperation(status.Operation.ResumeToken, func(state *sdkclients.OperationState) {
		state.Value = sdkclients.ClientCreateOrUpdateResponse{}
	})
	waitForDeploymentTemplateStateReadyPendingCleanup(t, c, key)
	deleting := waitForDeploymentResourceStateDeleting(t, c, outputKey, nil)

	require.NoError(t, c.Get(ctx, key, template))
	template.Spec.Parameters["revision"] = "reintroduced"
	require.NoError(t, c.Update(ctx, template))
	require.Never(t, func() bool {
		var current radappiov1alpha3.DeploymentTemplate
		require.NoError(t, c.Get(ctx, key, &current))
		return current.Status.Operation != nil || current.Status.Phrase != radappiov1alpha3.DeploymentTemplatePhraseReadyPendingCleanup
	}, time.Second, deploymentTemplateTestWaitInterval, "the manager must not submit a PUT while the old DELETE remains pending")

	backend.CompleteOperation(deleting.Operation.ResumeToken, nil)
	waitForDeploymentResourceDeleted(t, c, outputKey)
	status = waitForDeploymentTemplateStateUpdating(t, c, key, nil)
	backend.CompleteOperation(status.Operation.ResumeToken, completePut)
	waitForDeploymentTemplateStateReady(t, c, key)
	waitForDeploymentResourceStateReady(t, c, outputKey)
	var restored radappiov1alpha3.DeploymentResource
	require.NoError(t, c.Get(ctx, outputKey, &restored))
	require.Nil(t, restored.DeletionTimestamp)
	require.NotEqual(t, original.UID, restored.UID)
	require.NoError(t, c.Get(ctx, key, template))
	require.True(t, isUpToDate(template))
	require.True(t, isOwnedBy(restored, template))
	require.Equal(t, []string{outputID}, template.Status.OutputResources)

	require.NoError(t, c.Delete(ctx, template))
	deleting = waitForDeploymentResourceStateDeleting(t, c, outputKey, nil)
	backend.CompleteOperation(deleting.Operation.ResumeToken, nil)
	waitForDeploymentResourceDeleted(t, c, outputKey)
	waitForDeploymentTemplateStateDeleted(t, c, key)
}

func Test_DeploymentTemplateReconciler_DependencyCleanupUpdates(t *testing.T) {
	for _, parentType := range []string{"applications", "environments"} {
		for _, restoreParent := range []bool{false, true} {
			action := "remove"
			if restoreParent {
				action = "restore"
			}
			t.Run(parentType+"/"+action, func(t *testing.T) {
				_, backend, c := SetupDeploymentTemplateTest(t)
				ctx := t.Context()
				namespace := "dependency-" + parentType + "-" + action
				key := types.NamespacedName{Namespace: namespace, Name: "template"}
				require.NoError(t, c.Create(ctx, &corev1.Namespace{Name: namespace}))
				provider, err := sdkclients.NewDefaultProviderConfig(namespace).String()
				require.NoError(t, err)
				template := makeDeploymentTemplate(key, `{"resources":[]}`, provider, map[string]string{"revision": "initial"})
				require.NoError(t, c.Create(ctx, template))
				originalUID := template.UID
				parentID := "/planes/radius/local/resourceGroups/" + namespace + "/providers/Applications.Core/" + parentType + "/parent"
				childID := "/planes/radius/local/resourceGroups/" + namespace + "/providers/Applications.Core/containers/child"
				parentKey := types.NamespacedName{Namespace: namespace, Name: "parent"}
				childKey := types.NamespacedName{Namespace: namespace, Name: "child"}
				completePut := func(outputs []string) {
					t.Helper()
					status := waitForDeploymentTemplateStateUpdating(t, c, key, nil)
					backend.CompleteOperation(status.Operation.ResumeToken, func(state *sdkclients.OperationState) {
						refs := make([]*armdeployments.ResourceReference, 0, len(outputs))
						for _, id := range outputs {
							refs = append(refs, &armdeployments.ResourceReference{ID: new(id)})
						}
						state.Value = sdkclients.ClientCreateOrUpdateResponse{Properties: &armdeployments.DeploymentPropertiesExtended{OutputResources: refs}}
					})
				}
				completeDelete := func(resourceKey types.NamespacedName) {
					t.Helper()
					status := waitForDeploymentResourceStateDeleting(t, c, resourceKey, nil)
					backend.CompleteOperation(status.Operation.ResumeToken, nil)
					waitForDeploymentResourceDeleted(t, c, resourceKey)
				}
				completePut([]string{parentID, childID})
				waitForDeploymentTemplateStateReady(t, c, key)
				waitForDeploymentResourceStateReady(t, c, parentKey)
				waitForDeploymentResourceStateReady(t, c, childKey)
				var parent, child radappiov1alpha3.DeploymentResource
				require.NoError(t, c.Get(ctx, parentKey, &parent))
				require.NoError(t, c.Get(ctx, childKey, &child))
				require.NoError(t, c.Get(ctx, key, template))
				template.Spec.Parameters["revision"] = "parent removed"
				require.NoError(t, c.Update(ctx, template))
				completePut([]string{childID})
				waitForDeploymentTemplateStateReadyPendingCleanup(t, c, key)
				require.EventuallyWithT(t, func(check *assert.CollectT) {
					var pending radappiov1alpha3.DeploymentResource
					require.NoError(check, c.Get(ctx, parentKey, &pending))
					require.NotNil(check, pending.DeletionTimestamp)
					require.Nil(check, pending.Status.Operation)
				}, deploymentTemplateTestWaitDuration, deploymentTemplateTestWaitInterval)
				require.Empty(t, backend.DeletedResourceIDs())

				require.NoError(t, c.Get(ctx, key, template))
				template.Spec.Parameters["revision"] = "corrected"
				require.NoError(t, c.Update(ctx, template))
				if restoreParent {
					completePut([]string{parentID, childID})
					waitForDeploymentTemplateStateReady(t, c, key)
					require.EventuallyWithT(t, func(check *assert.CollectT) {
						var restored, retained radappiov1alpha3.DeploymentResource
						require.NoError(check, c.Get(ctx, parentKey, &restored))
						require.NoError(check, c.Get(ctx, childKey, &retained))
						require.NotEqual(check, parent.UID, restored.UID)
						require.Nil(check, restored.DeletionTimestamp)
						require.Equal(check, child.UID, retained.UID)
						require.Nil(check, retained.DeletionTimestamp)
					}, deploymentTemplateTestWaitDuration, deploymentTemplateTestWaitInterval)
					require.Empty(t, backend.DeletedResourceIDs(), "restoring the blocked parent must retain both backend resources")
				} else {
					completePut(nil)
					completeDelete(childKey)
					completeDelete(parentKey)
					waitForDeploymentTemplateStateReady(t, c, key)
					require.ElementsMatch(t, []string{childID, parentID}, backend.DeletedResourceIDs())
				}
				require.NoError(t, c.Get(ctx, key, template))
				require.Equal(t, originalUID, template.UID)
				require.Equal(t, template.Generation, template.Status.ObservedGeneration)
				require.True(t, isUpToDate(template))
				require.NoError(t, c.Delete(ctx, template))
				if restoreParent {
					completeDelete(childKey)
					completeDelete(parentKey)
				}
				waitForDeploymentTemplateStateDeleted(t, c, key)
			})
		}
	}
}

func Test_DeploymentTemplateReconciler_ReadyPendingCleanup(t *testing.T) {
	// After an update removes an output resource, the DT must transition to
	// ReadyPendingCleanup (not Ready) while the removed DR is still draining,
	// and only to Ready once the residual is gone.

	ctx := t.Context()

	_, mockDeploymentClient, k8sClient := SetupDeploymentTemplateTest(t)
	testNamespace := "deploymenttemplate-readypendingcleanup"
	testName := "test-deploymenttemplate-readypendingcleanup"
	template := readFileIntoTemplate(t, "deploymenttemplate-outputresources-1.json")
	parameters := map[string]string{}
	providerConfig, err := sdkclients.NewDefaultProviderConfig(testNamespace).String()
	require.NoError(t, err)

	namespacedName := types.NamespacedName{Namespace: testNamespace, Name: testName}
	err = k8sClient.Create(ctx, &corev1.Namespace{Name: testNamespace})
	require.NoError(t, err)

	deploymentTemplate := makeDeploymentTemplate(namespacedName, template, providerConfig, parameters)
	err = k8sClient.Create(ctx, deploymentTemplate)
	require.NoError(t, err)

	// First deployment: env + app + container.
	status := waitForDeploymentTemplateStateUpdating(t, k8sClient, namespacedName, nil)

	envID := "/planes/radius/local/resourceGroups/deploymenttemplate-readypendingcleanup/providers/Applications.Core/environments/deploymenttemplate-readypendingcleanup-environment"
	appID := "/planes/radius/local/resourceGroups/deploymenttemplate-readypendingcleanup/providers/Applications.Core/applications/deploymenttemplate-readypendingcleanup-application"
	containerID := "/planes/radius/local/resourceGroups/deploymenttemplate-readypendingcleanup/providers/Applications.Core/containers/deploymenttemplate-readypendingcleanup-container"

	mockDeploymentClient.CompleteOperation(status.Operation.ResumeToken, func(state *sdkclients.OperationState) {
		resource, ok := mockDeploymentClient.GetResource(state.ResourceID)
		require.True(t, ok, "failed to find resource")

		resource.Properties.OutputResources = []*armdeployments.ResourceReference{
			{ID: new(envID)},
			{ID: new(appID)},
			{ID: new(containerID)},
		}
		state.Value = sdkclients.ClientCreateOrUpdateResponse{Properties: resource.Properties}
	})

	_ = waitForDeploymentTemplateStateReady(t, k8sClient, namespacedName)

	containerDR := types.NamespacedName{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-readypendingcleanup-container"}
	_ = waitForDeploymentResourceStateReady(t, k8sClient, containerDR)

	// Re-deploy with the container removed.
	newDeploymentTemplate := radappiov1alpha3.DeploymentTemplate{}
	err = k8sClient.Get(ctx, namespacedName, &newDeploymentTemplate)
	require.NoError(t, err)
	newDeploymentTemplate.Spec.Template = readFileIntoTemplate(t, "deploymenttemplate-outputresources-2.json")
	err = k8sClient.Update(ctx, &newDeploymentTemplate)
	require.NoError(t, err)

	status = waitForDeploymentTemplateStateUpdating(t, k8sClient, namespacedName, nil)

	mockDeploymentClient.CompleteOperation(status.Operation.ResumeToken, func(state *sdkclients.OperationState) {
		resource, ok := mockDeploymentClient.GetResource(state.ResourceID)
		require.True(t, ok, "failed to find resource")

		resource.Properties.OutputResources = []*armdeployments.ResourceReference{
			{ID: new(envID)},
			{ID: new(appID)},
		}
		state.Value = sdkclients.ClientCreateOrUpdateResponse{Properties: resource.Properties}
	})

	// The container DR's delete operation is still pending in the mock, so
	// the DT should report ReadyPendingCleanup, not Ready.
	_ = waitForDeploymentTemplateStateReadyPendingCleanup(t, k8sClient, namespacedName)

	// Drain the container DR; the Owns watch should then wake the DT and
	// flip it to Ready.
	containerStatus := waitForDeploymentResourceStateDeleting(t, k8sClient, containerDR, nil)
	mockDeploymentClient.CompleteOperation(containerStatus.Operation.ResumeToken, nil)
	waitForDeploymentResourceDeleted(t, k8sClient, containerDR)

	_ = waitForDeploymentTemplateStateReady(t, k8sClient, namespacedName)

	// Cleanup.
	err = k8sClient.Delete(ctx, deploymentTemplate)
	require.NoError(t, err)

	waitForDeploymentTemplateStateDeleting(t, k8sClient, namespacedName)
	for _, dependency := range []types.NamespacedName{
		{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-readypendingcleanup-environment"},
		{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-readypendingcleanup-application"},
	} {
		dependencyStatus := waitForDeploymentResourceStateDeleting(t, k8sClient, dependency, nil)
		mockDeploymentClient.CompleteOperation(dependencyStatus.Operation.ResumeToken, nil)
		waitForDeploymentResourceDeleted(t, k8sClient, dependency)
	}
	waitForDeploymentTemplateStateDeleted(t, k8sClient, namespacedName)
}

func Test_DeploymentTemplateReconciler_NilOutputResourcesTriggersCleanup(t *testing.T) {
	// Regression for the case where a redeploy returns nil OutputResources for
	// a DT that previously tracked some. The diff/delete logic must still run
	// (treating nil as empty), otherwise residual DRs are never deleted and
	// pending resource cleanup holds the DT in ReadyPendingCleanup
	// indefinitely.

	ctx := t.Context()

	_, mockDeploymentClient, k8sClient := SetupDeploymentTemplateTest(t)
	testNamespace := "deploymenttemplate-niloutputs"
	testName := "test-deploymenttemplate-niloutputs"
	template := readFileIntoTemplate(t, "deploymenttemplate-outputresources-1.json")
	parameters := map[string]string{}
	providerConfig, err := sdkclients.NewDefaultProviderConfig(testNamespace).String()
	require.NoError(t, err)

	namespacedName := types.NamespacedName{Namespace: testNamespace, Name: testName}
	err = k8sClient.Create(ctx, &corev1.Namespace{Name: testNamespace})
	require.NoError(t, err)

	deploymentTemplate := makeDeploymentTemplate(namespacedName, template, providerConfig, parameters)
	err = k8sClient.Create(ctx, deploymentTemplate)
	require.NoError(t, err)

	// First deployment: env + app + container.
	status := waitForDeploymentTemplateStateUpdating(t, k8sClient, namespacedName, nil)

	envID := "/planes/radius/local/resourceGroups/deploymenttemplate-niloutputs/providers/Applications.Core/environments/deploymenttemplate-niloutputs-environment"
	appID := "/planes/radius/local/resourceGroups/deploymenttemplate-niloutputs/providers/Applications.Core/applications/deploymenttemplate-niloutputs-application"
	containerID := "/planes/radius/local/resourceGroups/deploymenttemplate-niloutputs/providers/Applications.Core/containers/deploymenttemplate-niloutputs-container"

	mockDeploymentClient.CompleteOperation(status.Operation.ResumeToken, func(state *sdkclients.OperationState) {
		resource, ok := mockDeploymentClient.GetResource(state.ResourceID)
		require.True(t, ok, "failed to find resource")

		resource.Properties.OutputResources = []*armdeployments.ResourceReference{
			{ID: new(envID)},
			{ID: new(appID)},
			{ID: new(containerID)},
		}
		state.Value = sdkclients.ClientCreateOrUpdateResponse{Properties: resource.Properties}
	})

	_ = waitForDeploymentTemplateStateReady(t, k8sClient, namespacedName)

	dependencies := []types.NamespacedName{
		// Order matters: container is the dependent of env/app via
		// checkForDeploymentResourceDependencies, so it must drain first.
		{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-niloutputs-container"},
		{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-niloutputs-application"},
		{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-niloutputs-environment"},
	}
	for _, dep := range dependencies {
		_ = waitForDeploymentResourceStateReady(t, k8sClient, dep)
	}

	// Re-deploy. We complete the operation WITHOUT setting OutputResources so
	// the response surfaces Properties != nil but Properties.OutputResources == nil
	// — the case the bug guards against.
	newDeploymentTemplate := radappiov1alpha3.DeploymentTemplate{}
	err = k8sClient.Get(ctx, namespacedName, &newDeploymentTemplate)
	require.NoError(t, err)
	newDeploymentTemplate.Spec.Template = readFileIntoTemplate(t, "deploymenttemplate-outputresources-2.json")
	err = k8sClient.Update(ctx, &newDeploymentTemplate)
	require.NoError(t, err)

	status = waitForDeploymentTemplateStateUpdating(t, k8sClient, namespacedName, nil)
	mockDeploymentClient.CompleteOperation(status.Operation.ResumeToken, nil)

	// All three DRs from the first generation must now be enqueued for delete.
	// Drain them so the residual check can converge.
	for _, dep := range dependencies {
		depStatus := waitForDeploymentResourceStateDeleting(t, k8sClient, dep, nil)
		mockDeploymentClient.CompleteOperation(depStatus.Operation.ResumeToken, nil)
		waitForDeploymentResourceDeleted(t, k8sClient, dep)
	}

	// DT must transition to Ready, not stick in ReadyPendingCleanup.
	_ = waitForDeploymentTemplateStateReady(t, k8sClient, namespacedName)

	// Cleanup.
	err = k8sClient.Delete(ctx, deploymentTemplate)
	require.NoError(t, err)
	waitForDeploymentTemplateStateDeleted(t, k8sClient, namespacedName)
}

func waitForDeploymentTemplateStateUpdating(t *testing.T, client k8sclient.Client, name types.NamespacedName, oldOperation *radappiov1alpha3.ResourceOperation) *radappiov1alpha3.DeploymentTemplateStatus {
	ctx := t.Context()

	logger := t
	status := &radappiov1alpha3.DeploymentTemplateStatus{}
	require.EventuallyWithT(t, func(t *assert.CollectT) {
		logger.Logf("Fetching DeploymentTemplate: %+v", name)
		current := &radappiov1alpha3.DeploymentTemplate{
			Status: radappiov1alpha3.DeploymentTemplateStatus{
				Phrase: radappiov1alpha3.DeploymentTemplatePhrase(radappiov1alpha3.DeploymentResourcePhraseDeleting),
			},
		}
		err := client.Get(ctx, name, current)
		require.NoError(t, err)

		status = &current.Status
		logger.Logf("DeploymentTemplate.Status: %+v", current.Status)
		assert.Equal(t, status.ObservedGeneration, current.Generation, "Status is not updated")

		if assert.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseUpdating, current.Status.Phrase) {
			assert.NotEmpty(t, current.Status.Operation)
			assert.NotEqual(t, oldOperation, current.Status.Operation)
		}

	}, deploymentTemplateTestWaitDuration, deploymentTemplateTestWaitInterval, "failed to enter updating state")

	return status
}

func waitForDeploymentTemplateStateReadyPendingCleanup(t *testing.T, client k8sclient.Client, name types.NamespacedName) *radappiov1alpha3.DeploymentTemplateStatus {
	ctx := t.Context()

	logger := t
	status := &radappiov1alpha3.DeploymentTemplateStatus{}
	require.EventuallyWithTf(t, func(t *assert.CollectT) {
		logger.Logf("Fetching DeploymentTemplate: %+v", name)
		current := &radappiov1alpha3.DeploymentTemplate{}
		err := client.Get(ctx, name, current)
		require.NoError(t, err)

		status = &current.Status
		logger.Logf("DeploymentTemplate.Status: %+v", current.Status)
		assert.Equal(t, status.ObservedGeneration, current.Generation, "Status is not updated")

		if assert.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseReadyPendingCleanup, current.Status.Phrase) {
			assert.Empty(t, current.Status.Operation)
		}
	}, deploymentTemplateTestWaitDuration, deploymentTemplateTestWaitInterval, "failed to enter ready-pending-cleanup state")

	return status
}

func waitForDeploymentTemplateStateReady(t *testing.T, client k8sclient.Client, name types.NamespacedName) *radappiov1alpha3.DeploymentTemplateStatus {
	ctx := t.Context()

	logger := t
	status := &radappiov1alpha3.DeploymentTemplateStatus{}
	require.EventuallyWithTf(t, func(t *assert.CollectT) {
		logger.Logf("Fetching DeploymentTemplate: %+v", name)
		current := &radappiov1alpha3.DeploymentTemplate{}
		err := client.Get(ctx, name, current)
		require.NoError(t, err)

		status = &current.Status
		logger.Logf("DeploymentTemplate.Status: %+v", current.Status)
		assert.Equal(t, status.ObservedGeneration, current.Generation, "Status is not updated")

		if assert.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseReady, current.Status.Phrase) {
			assert.Empty(t, current.Status.Operation)
		}
	}, deploymentTemplateTestWaitDuration, deploymentTemplateTestWaitInterval, "failed to enter ready state")

	return status
}

func waitForDeploymentTemplateStateDeleting(t *testing.T, client k8sclient.Client, name types.NamespacedName) *radappiov1alpha3.DeploymentTemplateStatus {
	ctx := t.Context()

	logger := t
	status := &radappiov1alpha3.DeploymentTemplateStatus{}
	require.EventuallyWithTf(t, func(t *assert.CollectT) {
		logger.Logf("Fetching DeploymentTemplate: %+v", name)
		current := &radappiov1alpha3.DeploymentTemplate{}
		err := client.Get(ctx, name, current)
		assert.NoError(t, err)

		status = &current.Status
		logger.Logf("DeploymentTemplate.Status: %+v", current.Status)
		assert.Equal(t, status.ObservedGeneration, current.Generation, "Status is not updated")

		assert.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseDeleting, current.Status.Phrase)
	}, deploymentTemplateTestWaitDuration, deploymentTemplateTestWaitInterval, "failed to enter deleting state")

	return status
}

func waitForDeploymentTemplateStateDeleted(t *testing.T, client k8sclient.Client, name types.NamespacedName) {
	ctx := t.Context()

	logger := t
	require.Eventuallyf(t, func() bool {
		logger.Logf("Fetching DeploymentTemplate: %+v", name)
		current := &radappiov1alpha3.DeploymentTemplate{}
		err := client.Get(ctx, name, current)
		if apierrors.IsNotFound(err) {
			return true
		}

		logger.Logf("DeploymentTemplate.Status: %+v", current.Status)
		return false

	}, deploymentTemplateTestWaitDuration, deploymentTemplateTestWaitInterval, "DeploymentTemplate still exists")
}

func readFileIntoTemplate(t *testing.T, filename string) string {
	fileContent, err := os.ReadFile(path.Join("testdata", filename))
	require.NoError(t, err)
	templateMap := map[string]any{}
	err = json.Unmarshal(fileContent, &templateMap)
	require.NoError(t, err)
	template, err := json.MarshalIndent(templateMap, "", "  ")
	require.NoError(t, err)
	return string(template)
}
