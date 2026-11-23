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

package helm

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/chart/common"
	valuesutil "helm.sh/helm/v4/pkg/chart/common/util"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/engine"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
)

func TestBootstrapNamespaceIntentBlocksDirectHelm(t *testing.T) {
	t.Parallel()
	for _, intent := range []struct{ key, value string }{
		{bootstrapNamespaceIntent, ""}, {bootstrapNamespaceIntent, "unresolved-intent"},
		{initialCertificateIntent, "unresolved-intent"},
	} {
		t.Run(intent.key+"/"+intent.value, func(t *testing.T) {
			t.Parallel()
			fixture := newCertificateChartFixture(t)
			fixture.objects[0].SetAnnotations(map[string]string{intent.key: intent.value})
			selected, err := loader.Load("../../../deploy/Chart")
			require.NoError(t, err)
			server := fixture.server(t)
			renderer := engine.New(&rest.Config{Host: server.URL})
			_, err = renderer.RenderWithContext(t.Context(), selected, common.Values{"Values": selected.Values, "Chart": selected.Metadata,
				"Capabilities": common.DefaultCapabilities,
				"Release":      map[string]any{"Name": radiusReleaseName, "Namespace": RadiusSystemNamespace, "Service": "Helm", "IsInstall": true}})
			require.ErrorContains(t, err, "bootstrap intent without its journal")
		})
	}
}

func TestLegacyBootstrapJournalRejectedBySelectedChart(t *testing.T) {
	const bootstrapInvocationValue = "aggregationBootstrapInvocation"
	for _, mode := range []string{"selfSigned", "caSecret", "issuerRef"} {
		for _, external := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/external=%t", mode, external), func(t *testing.T) {
				s := bootstrapStoreFixture(t)
				fixture := newCertificateChartFixture(t)
				selected, err := loader.Load("../../../deploy/Chart")
				require.NoError(t, err)
				ca, _, _ := testSigningCA(t, nil, nil, nil, nil)
				values := copyValues(fixture.values)
				provider := "radius"
				if external {
					provider = "external"
				}
				require.NoError(t, unstructured.SetNestedField(values, provider, "global", "rbac", "tls", "provider"))
				settings := map[string]any{"mode": mode}
				if mode == "caSecret" {
					settings["caSecret"], settings["caBundle"] = "operator-ca", string(ca)
				} else if mode == "issuerRef" {
					settings = certificateValues(values)
					settings["caBundle"] = string(ca)
				}
				setCertificateValues(values, settings)
				values, err = valuesutil.CoalesceValues(selected, values)
				require.NoError(t, err)
				require.NoError(t, unstructured.SetNestedField(values, "lease-uid", "global", "rbac", bootstrapInvocationValue))
				journal := &unstructured.Unstructured{Object: map[string]any{
					"apiVersion": "v1", "kind": "ConfigMap",
					"metadata": map[string]any{"name": aggregationBootstrapName, "namespace": s.namespace,
						"uid": "bootstrap-uid", "resourceVersion": "1", "labels": map[string]any{aggregationBootstrapOwner: s.release}},
					"data": map[string]any{rotationJournalKey: `{"version":1,"phase":"complete"}`},
				}}
				fixture.objects = append(fixture.objects, journal, &unstructured.Unstructured{Object: map[string]any{
					"apiVersion": "coordination.k8s.io/v1", "kind": "Lease",
					"metadata": map[string]any{"name": identityOperationLeaseName, "namespace": s.namespace, "uid": "lease-uid", "resourceVersion": "1"},
				}})
				server := fixture.server(t)
				renderer := engine.New(&rest.Config{Host: server.URL})
				render := func() (map[string]string, error) {
					return renderer.RenderWithContext(t.Context(), selected, common.Values{"Values": values, "Chart": selected.Metadata,
						"Capabilities": common.DefaultCapabilities,
						"Release":      map[string]any{"Name": s.release, "Namespace": s.namespace, "Service": "Helm", "IsInstall": true}})
				}
				_, err = render()
				require.ErrorContains(t, err, "Legacy aggregation bootstrap journal")
				unstructured.RemoveNestedField(values, "global", "rbac", bootstrapInvocationValue)
				_, err = render()
				require.ErrorContains(t, err, "Legacy aggregation bootstrap journal")
				require.NoError(t, unstructured.SetNestedField(values, "lease-uid", "global", "rbac", bootstrapInvocationValue))
				require.NoError(t, unstructured.SetNestedField(values, "different-image", "ucp", "image"))
				_, err = render()
				require.ErrorContains(t, err, "Legacy aggregation bootstrap journal")
			})
		}
	}
}
