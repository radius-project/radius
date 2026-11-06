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
	"context"
	"errors"
	"os"
	"testing"
	"time"

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/yaml"
)

func TestDeploymentResourceSchemaUpgrade(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("KUBEBUILDER_ASSETS is required for the schema upgrade regression")
	}

	ctx := t.Context()
	data, err := os.ReadFile("../../deploy/Chart/crds/radius/radapp.io_deploymentresources.yaml")
	require.NoError(t, err)
	crd := &apiextensionsv1.CustomResourceDefinition{}
	require.NoError(t, yaml.Unmarshal(data, crd))
	status := crd.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["status"]
	delete(status.Properties, "provisionedResourceID")
	env := &envtest.Environment{CRDs: []*apiextensionsv1.CustomResourceDefinition{crd}}
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
	require.NoError(t, ensureDeploymentResourceSchema(ctx, c))
	require.Eventually(t, func() bool {
		if err := c.Get(ctx, client.ObjectKeyFromObject(child), child); err != nil {
			return false
		}
		child.Status.ProvisionedResourceID = child.Spec.Id
		return c.Status().Update(ctx, child) == nil && child.Status.ProvisionedResourceID == child.Spec.Id
	}, 10*time.Second, 100*time.Millisecond, "migrated schema must preserve provenance on existing children")
	require.NoError(t, c.Get(ctx, client.ObjectKey{Name: crd.Name}, crd))
	rv := crd.ResourceVersion
	require.NoError(t, ensureDeploymentResourceSchema(ctx, c))
	require.NoError(t, c.Get(ctx, client.ObjectKey{Name: crd.Name}, crd))
	require.Equal(t, rv, crd.ResourceVersion, "migration must be idempotent")
	require.Contains(t, crd.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["status"].Properties, "operation", "preserve unrelated schema")
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(child), child))
	require.Equal(t, child.Spec.Id, child.Status.ProvisionedResourceID)
}

func TestDeploymentResourceSchemaUpgradeFailure(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../deploy/Chart/crds/radius/radapp.io_deploymentresources.yaml")
	require.NoError(t, err)
	crd := &apiextensionsv1.CustomResourceDefinition{}
	require.NoError(t, yaml.Unmarshal(data, crd))
	status := crd.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["status"]
	delete(status.Properties, "provisionedResourceID")
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crd).WithInterceptorFuncs(interceptor.Funcs{
		Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
			return errors.New("schema patch denied")
		},
	}).Build()
	require.ErrorContains(t, ensureDeploymentResourceSchema(t.Context(), c), "schema patch denied")
}
