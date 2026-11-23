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
	"testing"

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

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
