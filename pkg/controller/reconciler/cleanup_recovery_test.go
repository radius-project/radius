/*
Copyright 2026 The Radius Authors.

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
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armdeployments/v3"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type cleanupTestDeployments struct {
	*sdkclients.MockResourceDeploymentsClient
	deleted   []string
	deleteErr error
}

func (c *cleanupTestDeployments) Delete(ctx context.Context, id, version string) (sdkclients.Poller[sdkclients.ClientDeleteResponse], error) {
	if c.deleteErr != nil {
		return nil, c.deleteErr
	}
	c.deleted = append(c.deleted, id)
	return c.MockResourceDeploymentsClient.Delete(ctx, id, version)
}

func Test_DeploymentTemplateReconciler_ObsoleteOutputLiveRead(t *testing.T) {
	t.Parallel()
	id := "/planes/radius/local/resourceGroups/target/providers/Applications.Core/containers/obsolete"
	template := &radappiov1alpha3.DeploymentTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "template", Namespace: "target", UID: "template-uid"},
		Status:     radappiov1alpha3.DeploymentTemplateStatus{OutputResources: []string{id}},
	}
	output := &radappiov1alpha3.DeploymentResource{
		ObjectMeta: metav1.ObjectMeta{Name: "obsolete", Namespace: template.Namespace, Finalizers: []string{DeploymentResourceFinalizer},
			OwnerReferences: []metav1.OwnerReference{{Kind: deploymentTemplateKind, Name: template.Name, UID: template.UID, Controller: new(true)}}},
		Spec: radappiov1alpha3.DeploymentResourceSpec{Id: id},
	}
	backend := &cleanupTestDeployments{MockResourceDeploymentsClient: sdkclients.NewMockResourceDeploymentsClient()}
	poller, err := backend.CreateOrUpdate(t.Context(), sdkclients.Deployment{}, "completed", "test")
	require.NoError(t, err)
	token, err := poller.ResumeToken()
	require.NoError(t, err)
	backend.CompleteOperation(token, nil)
	template.Status.Operation = &radappiov1alpha3.ResourceOperation{OperationKind: radappiov1alpha3.OperationKindPut, ResumeToken: token}
	live := fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).
		WithStatusSubresource(&radappiov1alpha3.DeploymentTemplate{}).WithObjects(template, output).Build()
	cache := interceptor.NewClient(live, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, object client.Object, opts ...client.GetOption) error {
			if _, ok := object.(*radappiov1alpha3.DeploymentResource); ok {
				return apierrors.NewNotFound(radappiov1alpha3.GroupVersion.WithResource("deploymentresources").GroupResource(), key.Name)
			}
			return c.Get(ctx, key, object, opts...)
		},
	})
	policy, err := NewFluxSourcePolicy(hostoptions.FluxOptions{}, live)
	require.NoError(t, err)
	r := &DeploymentTemplateReconciler{Client: cache, SourcePolicy: policy, ResourceDeploymentsClient: backend}
	_, err = r.reconcileOperation(t.Context(), template)
	require.NoError(t, err)
	require.NoError(t, live.Get(t.Context(), client.ObjectKeyFromObject(output), output))
	require.NotNil(t, output.DeletionTimestamp, "a stale cached miss must not strand an obsolete output")
}

func Test_DeploymentTemplateReconciler_TerminatingNamespace(t *testing.T) {
	SkipWithoutEnvironment(t)
	for _, tt := range []struct {
		name          string
		trackedChild  bool
		trackedParent bool
	}{
		{name: "existing outputs", trackedChild: true, trackedParent: true},
		{name: "missing outputs"},
		{name: "missing child", trackedParent: true},
		{name: "missing parent", trackedChild: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, err := client.New(config, client.Options{Scheme: scheme})
			require.NoError(t, err)
			namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "cleanup-"}}
			require.NoError(t, c.Create(t.Context(), namespace))
			provider, err := sdkclients.NewDefaultProviderConfig("target").String()
			require.NoError(t, err)
			template := &radappiov1alpha3.DeploymentTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "template", Namespace: namespace.Name, Finalizers: []string{DeploymentTemplateFinalizer}},
				Spec:       radappiov1alpha3.DeploymentTemplateSpec{Template: "{}", ProviderConfig: provider},
			}
			require.NoError(t, c.Create(t.Context(), template))
			id := "/planes/radius/local/resourceGroups/target/providers/Applications.Core/containers/output"
			output := &radappiov1alpha3.DeploymentResource{
				ObjectMeta: metav1.ObjectMeta{Name: "output", Namespace: namespace.Name, Finalizers: []string{DeploymentResourceFinalizer}},
				Spec:       radappiov1alpha3.DeploymentResourceSpec{Id: id},
			}
			require.NoError(t, controllerutil.SetControllerReference(template, output, scheme))
			if tt.trackedChild {
				require.NoError(t, c.Create(t.Context(), output))
			}
			parentID := "/planes/radius/local/resourceGroups/target/providers/Applications.Core/applications/parent"
			parent := output.DeepCopy()
			parent.Name, parent.UID, parent.ResourceVersion = "parent", "", ""
			parent.Spec.Id = parentID
			if tt.trackedParent {
				require.NoError(t, c.Create(t.Context(), parent))
			}
			backend := &cleanupTestDeployments{MockResourceDeploymentsClient: sdkclients.NewMockResourceDeploymentsClient()}
			poller, err := backend.CreateOrUpdate(t.Context(), sdkclients.Deployment{}, "completed", "test")
			require.NoError(t, err)
			token, err := poller.ResumeToken()
			require.NoError(t, err)
			backend.CompleteOperation(token, func(state *sdkclients.OperationState) {
				state.Value = sdkclients.ClientCreateOrUpdateResponse{Properties: &armdeployments.DeploymentPropertiesExtended{
					OutputResources: []*armdeployments.ResourceReference{{ID: new(parentID)}, {ID: new(id)}},
				}}
			})
			template.Status.Operation = &radappiov1alpha3.ResourceOperation{OperationKind: radappiov1alpha3.OperationKindPut, ResumeToken: token}
			require.NoError(t, c.Status().Update(t.Context(), template))
			require.NoError(t, c.Delete(t.Context(), template))
			require.NoError(t, c.Delete(t.Context(), namespace))
			require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(template), template))
			require.True(t, apierrors.IsForbidden(c.Create(t.Context(), output)), "real NamespaceLifecycle admission must reject creates")
			policy, err := NewFluxSourcePolicy(hostoptions.FluxOptions{}, c)
			require.NoError(t, err)
			r := &DeploymentTemplateReconciler{Client: c, Scheme: scheme, SourcePolicy: policy,
				EventRecorder: record.NewFakeRecorder(10), ResourceDeploymentsClient: backend}
			_, err = r.reconcileOperation(t.Context(), template)
			require.NoError(t, err, "completion must not create namespace-local objects")
			require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(template), template))
			require.Nil(t, template.Status.Operation)
			require.Contains(t, template.Finalizers, DeploymentTemplateFinalizer)
			expectedPending := 2
			if tt.trackedChild {
				expectedPending--
			}
			if tt.trackedParent {
				expectedPending--
			}
			require.Len(t, template.Status.PendingCleanup, expectedPending, "missing outputs must survive API serialization")
			resources := &DeploymentResourceReconciler{Client: c, SourcePolicy: policy,
				EventRecorder: record.NewFakeRecorder(20), ResourceDeploymentsClient: backend}
			for attempt := 0; attempt < 12; attempt++ {
				// Reconstruct the controller each time so progress depends on stored
				// state rather than a remembered operation or in-memory inventory.
				r = &DeploymentTemplateReconciler{Client: c, Scheme: scheme, SourcePolicy: policy,
					EventRecorder: record.NewFakeRecorder(20), ResourceDeploymentsClient: backend}
				_, err = r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(template)})
				if err != nil {
					require.ErrorContains(t, err, "owned resources")
				}
				err = c.Get(t.Context(), client.ObjectKeyFromObject(template), template)
				if apierrors.IsNotFound(err) {
					break
				}
				require.NoError(t, err)
				for _, item := range template.Status.PendingCleanup {
					if item.Operation != nil {
						backend.CompleteOperation(item.Operation.ResumeToken, nil)
					}
				}
				for _, object := range []*radappiov1alpha3.DeploymentResource{parent, output} {
					key := client.ObjectKeyFromObject(object)
					_, err = resources.Reconcile(t.Context(), ctrl.Request{NamespacedName: key})
					require.NoError(t, err)
					err = c.Get(t.Context(), key, object)
					if apierrors.IsNotFound(err) {
						continue
					}
					require.NoError(t, err)
					if object.Status.Operation != nil {
						backend.CompleteOperation(object.Status.Operation.ResumeToken, nil)
					}
				}
			}
			require.True(t, apierrors.IsNotFound(c.Get(t.Context(), client.ObjectKeyFromObject(template), template)))
			require.Equal(t, []string{id, parentID}, backend.deleted, "children must drain before parents, including missing trackers")
			var remaining radappiov1alpha3.DeploymentResourceList
			require.NoError(t, c.List(t.Context(), &remaining, client.InNamespace(namespace.Name)))
			require.Empty(t, remaining.Items)
		})
	}
}

func Test_DeploymentTemplateReconciler_PendingCleanupRecovery(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"source hold", "schema pruning", "inventory write failure", "delete start failure", "delete failure", "unknown delete token", "outside scope"} {
		t.Run(scenario, func(t *testing.T) {
			source := readyFluxSource()
			provider, err := sdkclients.NewDefaultProviderConfig("target").String()
			require.NoError(t, err)
			template := &radappiov1alpha3.DeploymentTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "template", Namespace: "target", UID: "template-uid",
					DeletionTimestamp: new(metav1.Now()), Finalizers: []string{DeploymentTemplateFinalizer}},
				Spec: radappiov1alpha3.DeploymentTemplateSpec{Template: "{}", ProviderConfig: provider,
					Repository: source.Name, SourceRef: sourceReference(source)},
				Status: radappiov1alpha3.DeploymentTemplateStatus{
					Operation: &radappiov1alpha3.ResourceOperation{OperationKind: radappiov1alpha3.OperationKindPut, ResumeToken: "completed"},
				},
			}
			id := "/planes/radius/local/resourceGroups/target/providers/Applications.Core/containers/output"
			base := fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).
				WithStatusSubresource(&radappiov1alpha3.DeploymentTemplate{}).WithObjects(source, template).Build()
			injectFailure := true
			c := interceptor.NewClient(base, interceptor.Funcs{
				SubResourceUpdate: func(ctx context.Context, c client.Client, subResource string, object client.Object, opts ...client.SubResourceUpdateOption) error {
					if injectFailure {
						if scenario == "inventory write failure" {
							return errors.New("inventory write failed")
						}
						if scenario == "schema pruning" {
							object.(*radappiov1alpha3.DeploymentTemplate).Status.PendingCleanup = nil
						}
					}
					return c.SubResource(subResource).Update(ctx, object, opts...)
				},
			})
			backend := &cleanupTestDeployments{MockResourceDeploymentsClient: sdkclients.NewMockResourceDeploymentsClient()}
			policy := selectedSourcePolicy(t, base, source)
			r := &DeploymentTemplateReconciler{Client: c, SourcePolicy: policy, Scheme: newFluxPolicyTestScheme(t),
				EventRecorder: record.NewFakeRecorder(20), ResourceDeploymentsClient: backend}
			err = r.recoverTerminatingOutputs(t.Context(), template, []string{id, id})
			if scenario == "inventory write failure" || scenario == "schema pruning" {
				require.Error(t, err)
				require.NoError(t, base.Get(t.Context(), client.ObjectKeyFromObject(template), template))
				require.NotNil(t, template.Status.Operation, "failed inventory persistence must retain the PUT result")
				injectFailure = false
				require.NoError(t, r.recoverTerminatingOutputs(t.Context(), template, []string{id, id}))
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, base.Get(t.Context(), client.ObjectKeyFromObject(template), template))
			require.Len(t, template.Status.PendingCleanup, 1)
			if scenario == "outside scope" {
				template.Spec.ProviderConfig, err = sdkclients.NewDefaultProviderConfig("different").String()
				require.NoError(t, err)
				require.NoError(t, base.Update(t.Context(), template))
				_, err = r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(template)})
				require.NoError(t, err)
				require.Empty(t, backend.deleted)
				require.NoError(t, base.Get(t.Context(), client.ObjectKeyFromObject(template), template))
				require.Len(t, template.Status.PendingCleanup, 1)
				require.Contains(t, template.Finalizers, DeploymentTemplateFinalizer)
				return
			}
			if scenario == "delete start failure" {
				backend.deleteErr = errors.New("delete request failed")
				_, err = r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(template)})
				require.ErrorContains(t, err, "delete request failed")
				require.NoError(t, base.Get(t.Context(), client.ObjectKeyFromObject(template), template))
				require.Len(t, template.Status.PendingCleanup, 1)
				backend.deleteErr = nil
			}
			if scenario == "source hold" {
				source.Spec.Suspend = true
				require.NoError(t, base.Update(t.Context(), source))
				_, err = r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(template)})
				require.NoError(t, err)
				require.Empty(t, backend.deleted)
				require.NoError(t, base.Get(t.Context(), client.ObjectKeyFromObject(template), template))
				require.Len(t, template.Status.PendingCleanup, 1)
				source.Spec.Suspend = false
				require.NoError(t, base.Update(t.Context(), source))
			}
			_, err = r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(template)})
			require.NoError(t, err)
			require.NoError(t, base.Get(t.Context(), client.ObjectKeyFromObject(template), template))
			token := template.Status.PendingCleanup[0].Operation.ResumeToken
			if scenario == "unknown delete token" {
				template.Status.PendingCleanup[0].Operation.ResumeToken = "missing-operation"
				require.NoError(t, base.Status().Update(t.Context(), template))
				_, err = r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(template)})
				require.ErrorContains(t, err, "continuing untracked")
				require.NoError(t, base.Get(t.Context(), client.ObjectKeyFromObject(template), template))
				require.Len(t, template.Status.PendingCleanup, 1, "an unknown operation is not proof of resource deletion")
				template.Status.PendingCleanup[0].Operation.ResumeToken = token
				require.NoError(t, base.Status().Update(t.Context(), template))
			}
			backend.CompleteOperation(token, func(state *sdkclients.OperationState) {
				if scenario == "delete failure" {
					state.Err = errors.New("backend deletion failed")
				}
			})
			_, err = r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(template)})
			if scenario == "delete failure" {
				require.ErrorContains(t, err, "backend deletion failed")
				require.NoError(t, base.Get(t.Context(), client.ObjectKeyFromObject(template), template))
				require.Len(t, template.Status.PendingCleanup, 1)
				require.Nil(t, template.Status.PendingCleanup[0].Operation)
				_, err = r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(template)})
				require.NoError(t, err)
				require.NoError(t, base.Get(t.Context(), client.ObjectKeyFromObject(template), template))
				backend.CompleteOperation(template.Status.PendingCleanup[0].Operation.ResumeToken, nil)
				_, err = r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(template)})
			}
			require.NoError(t, err)
			_, err = r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(template)})
			require.NoError(t, err)
			require.True(t, apierrors.IsNotFound(base.Get(t.Context(), client.ObjectKeyFromObject(template), template)))
		})
	}
}

func Test_FluxController_CleanupAttributionGuards(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"replaced target", "active target", "different source", "suspended source"} {
		t.Run(scenario, func(t *testing.T) {
			source := readyFluxSource()
			template := &radappiov1alpha3.DeploymentTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "app.bicep", Namespace: "target", UID: "template-uid",
					DeletionTimestamp: new(metav1.Now()), Finalizers: []string{DeploymentTemplateFinalizer}},
				Spec: radappiov1alpha3.DeploymentTemplateSpec{Template: "old content", Repository: source.Name, SourceRef: sourceReference(source)},
			}
			uid := template.UID
			source.Generation++
			source.Status.ObservedGeneration = source.Generation
			source.Status.Conditions[0].ObservedGeneration = source.Generation
			switch scenario {
			case "replaced target":
				template.UID = "replacement"
			case "active target":
				template.DeletionTimestamp = nil
			case "different source":
				template.Spec.SourceRef.UID = "another-source"
			case "suspended source":
				source.Spec.Suspend = true
			}
			c := fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).WithObjects(source, template).Build()
			r := &FluxController{Client: c, SourcePolicy: selectedSourcePolicy(t, c, source)}
			err := r.refreshCleanupAttribution(t.Context(), client.ObjectKeyFromObject(template), uid, sourceReference(source))
			var hold *sourceHold
			require.ErrorAs(t, err, &hold)
			require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(template), template))
			require.Equal(t, int64(1), template.Spec.SourceRef.Generation)
			require.Equal(t, "old content", template.Spec.Template)
		})
	}
}

func Test_FluxController_ConfiguredDeletingTarget(t *testing.T) {
	t.Parallel()
	source := readyFluxSource()
	template := &radappiov1alpha3.DeploymentTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "app.bicep", Namespace: "target", UID: "template-uid",
			Finalizers: []string{DeploymentTemplateFinalizer}},
		Spec: radappiov1alpha3.DeploymentTemplateSpec{Template: "old content", Repository: source.Name, SourceRef: sourceReference(source)},
	}
	c := fake.NewClientBuilder().WithScheme(newFluxPolicyTestScheme(t)).
		WithStatusSubresource(&radappiov1alpha3.DeploymentTemplate{}).WithObjects(source, template).Build()
	policy := selectedSourcePolicy(t, c, source)
	r := &FluxController{Client: c, SourcePolicy: policy}
	source.Spec.Suspend = true
	source.Generation++
	require.NoError(t, c.Update(t.Context(), source))
	require.NoError(t, c.Delete(t.Context(), template))
	source.Spec.Suspend = false
	source.Generation++
	source.Status.ObservedGeneration = source.Generation
	source.Status.Conditions[0].ObservedGeneration = source.Generation
	require.NoError(t, c.Update(t.Context(), source))
	_, err := r.preflightTargets(t.Context(), []ConfigEntry{{Name: template.Name, Namespace: template.Namespace}}, sourceReference(source))
	var hold *sourceHold
	require.ErrorAs(t, err, &hold)
	require.Equal(t, "TemplateDeleting", hold.reason)
	require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(template), template))
	require.Equal(t, source.Generation, template.Spec.SourceRef.Generation)
	require.Equal(t, "old content", template.Spec.Template)
	require.NotNil(t, template.DeletionTimestamp)
	require.NoError(t, policy.checkTemplate(t.Context(), template), "only cleanup may resume under the new generation")
	backend := &sourceTestDeployments{ResourceDeploymentsClient: sdkclients.NewMockResourceDeploymentsClient()}
	cleanup := &DeploymentTemplateReconciler{Client: c, SourcePolicy: policy, EventRecorder: record.NewFakeRecorder(5), ResourceDeploymentsClient: backend}
	_, err = cleanup.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(template)})
	require.NoError(t, err)
	require.Zero(t, backend.creates)
}
