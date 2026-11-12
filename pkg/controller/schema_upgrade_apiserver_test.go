//go:build !windows

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

package controller

import (
	"os"
	"testing"
	"time"

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestDeploymentResourceSchemaUpgrade(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("KUBEBUILDER_ASSETS is required for the schema upgrade regression")
	}

	ctx := t.Context()
	crd, templateCRD := deploymentStatusCRDs(t)
	status := crd.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["status"]
	delete(status.Properties, "provisionedResourceID")
	delete(templateCRD.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["status"].Properties, "coordinator")
	env := &envtest.Environment{CRDs: []*apiextensionsv1.CustomResourceDefinition{crd, templateCRD}}
	cfg, err := env.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, env.Stop()) })
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	require.NoError(t, err)
	require.NoError(t, c.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "schema-upgrade"}}))
	child := &radappiov1alpha3.DeploymentResource{
		ObjectMeta: metav1.ObjectMeta{Namespace: "schema-upgrade", Name: "legacy"},
		Spec:       radappiov1alpha3.DeploymentResourceSpec{Id: "/planes/kubernetes/local/namespaces/schema-upgrade/providers/core/ConfigMap/output"},
	}
	require.NoError(t, c.Create(ctx, child))
	child.Status.ProvisionedResourceID = child.Spec.Id
	require.NoError(t, c.Status().Update(ctx, child))
	require.Empty(t, child.Status.ProvisionedResourceID, "old schema must reproduce status pruning")
	template := &radappiov1alpha3.DeploymentTemplate{
		ObjectMeta: metav1.ObjectMeta{Namespace: "schema-upgrade", Name: "legacy"},
	}
	require.NoError(t, c.Create(ctx, template))
	coordinator := &radappiov1alpha3.DeploymentCoordinatorStatus{
		Phase: "DeleteDispatching",
		Outputs: []radappiov1alpha3.DeploymentOutputStatus{
			{ID: child.Spec.Id, State: "PendingDelete"},
			{ID: child.Spec.Id + "-current", State: "Current"},
		},
		ResourceID:    child.Spec.Id,
		SubmittedHash: "submitted-template-hash",
		Message:       "request outcome is unknown",
		RetryAfter:    &metav1.Time{Time: time.Date(2026, 11, 9, 23, 0, 0, 0, time.UTC)},
	}
	template.Status.Coordinator = coordinator.DeepCopy()
	template.Status.Operation = &radappiov1alpha3.ResourceOperation{ResumeToken: "existing-resume-token"}
	require.NoError(t, c.Status().Update(ctx, template))
	require.Nil(t, template.Status.Coordinator, "old template schema must reproduce coordinator pruning")
	require.Equal(t, "existing-resume-token", template.Status.Operation.ResumeToken)
	require.NoError(t, ensureDeploymentResourceSchema(ctx, c))
	require.Eventually(t, func() bool {
		if err := c.Get(ctx, client.ObjectKeyFromObject(child), child); err != nil {
			return false
		}
		child.Status.ProvisionedResourceID = child.Spec.Id
		return c.Status().Update(ctx, child) == nil && child.Status.ProvisionedResourceID == child.Spec.Id
	}, 10*time.Second, 100*time.Millisecond, "migrated schema must preserve provenance on existing children")
	require.Eventually(t, func() bool {
		if err := c.Get(ctx, client.ObjectKeyFromObject(template), template); err != nil {
			return false
		}
		template.Status.Coordinator = coordinator.DeepCopy()
		return c.Status().Update(ctx, template) == nil && template.Status.Coordinator != nil
	}, 10*time.Second, 100*time.Millisecond, "migrated schema must preserve coordinator intent on existing templates")
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(template), template))
	require.NotNil(t, template.Status.Coordinator.RetryAfter)
	template.Status.Coordinator.RetryAfter.Time = template.Status.Coordinator.RetryAfter.UTC()
	require.Equal(t, coordinator, template.Status.Coordinator)
	require.Equal(t, "existing-resume-token", template.Status.Operation.ResumeToken)
	require.NoError(t, c.Get(ctx, client.ObjectKey{Name: crd.Name}, crd))
	require.NoError(t, c.Get(ctx, client.ObjectKey{Name: templateCRD.Name}, templateCRD))
	rv := crd.ResourceVersion
	templateRV := templateCRD.ResourceVersion
	require.NoError(t, ensureDeploymentResourceSchema(ctx, c))
	require.NoError(t, c.Get(ctx, client.ObjectKey{Name: crd.Name}, crd))
	require.Equal(t, rv, crd.ResourceVersion, "migration must be idempotent")
	require.NoError(t, c.Get(ctx, client.ObjectKey{Name: templateCRD.Name}, templateCRD))
	require.Equal(t, templateRV, templateCRD.ResourceVersion, "template migration must be idempotent")
	require.Contains(t, crd.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["status"].Properties, "operation", "preserve unrelated schema")
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(child), child))
	require.Equal(t, child.Spec.Id, child.Status.ProvisionedResourceID)
}
