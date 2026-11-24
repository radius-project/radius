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
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/chart/common"
	valuesutil "helm.sh/helm/v4/pkg/chart/common/util"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/engine"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
)

func TestPublishedIssuerJournalConnectedRender(t *testing.T) {
	b, _ := initialControllerFixture(t, "issuerRef", "external")
	b.applyFixture = func(ctx context.Context) error {
		fixture := newCertificateChartFixture(t)
		fixture.values = b.values
		fixture.objects[0].SetUID(b.state.NamespaceUID)
		issuer, err := b.store.dynamic.Resource(rotationIssuerResource).Namespace(b.store.namespace).Get(ctx, "operator-issuer", metav1.GetOptions{})
		require.NoError(t, err)
		issuer.SetGeneration(1)
		issuer.Object["status"] = fixture.objects[1].Object["status"]
		fixture.objects[1] = issuer
		src, err := b.store.kube.CoreV1().Secrets(b.store.namespace).Get(ctx, ucpAPIServerTrustSecretName, metav1.GetOptions{})
		require.NoError(t, err)
		for kind, object := range map[string]any{"Secret": src, "ConfigMap": b.journal} {
			data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
			require.NoError(t, err)
			data["apiVersion"], data["kind"] = "v1", kind
			fixture.objects = append(fixture.objects, &unstructured.Unstructured{Object: data})
		}
		_, err = fixture.renderChart(t)
		return err
	}
	require.NoError(t, b.run(t.Context()))
}

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

func TestProviderContinuityConnectedUpgrade(t *testing.T) {
	for _, old := range []string{"radius", "external"} {
		for _, next := range []string{"radius", "external"} {
			for _, mode := range []string{"enforce", "dryRun", "off"} {
				t.Run(old+"/"+next+"/"+mode, func(t *testing.T) {
					fixture := newCertificateChartFixture(t)
					selected, err := loader.Load("../../../deploy/Chart")
					require.NoError(t, err)
					values := copyValues(fixture.values)
					setValue(values, mode == "enforce", "global", "rbac", "enabled")
					setValue(values, mode == "dryRun", "global", "rbac", "dryRun")
					setValue(values, next, "global", "rbac", "tls", "provider")
					values, err = valuesutil.CoalesceValues(selected, values)
					require.NoError(t, err)
					fixture.objects = append(fixture.objects, &unstructured.Unstructured{Object: map[string]any{
						"apiVersion": "v1", "kind": "ConfigMap",
						"metadata": map[string]any{"name": "ucp-config", "namespace": RadiusSystemNamespace,
							"uid": "config-uid", "resourceVersion": "1",
							"labels": map[string]any{"app.kubernetes.io/managed-by": "Helm"},
							"annotations": map[string]any{"meta.helm.sh/release-name": radiusReleaseName,
								"meta.helm.sh/release-namespace": RadiusSystemNamespace, "radius.io/tls-provider": old}},
						"data": map[string]any{"ucp-config.yaml": "authorization:\n  mode: enforce\n  tlsProvider: " + old + "\n"},
					}})
					server := fixture.server(t)
					renderer := engine.New(&rest.Config{Host: server.URL})
					rendered, err := renderer.RenderWithContext(t.Context(), selected, common.Values{
						"Values": values, "Chart": selected.Metadata, "Capabilities": common.DefaultCapabilities,
						"Release": map[string]any{"Name": radiusReleaseName, "Namespace": RadiusSystemNamespace, "Service": "Helm", "IsUpgrade": true},
					})
					if old != next {
						require.ErrorContains(t, err, "TLS provider transition")
						require.Empty(t, rendered)
					} else {
						require.NoError(t, err)
					}
				})
			}
		}
	}
}

func TestProviderContinuityLegacyAndOwnership(t *testing.T) {
	for _, tc := range []struct {
		name, raw, record, want string
	}{
		{name: "legacy radius", raw: "authorization:\n  mode: enforce\n  tlsProvider: radius\n"},
		{name: "legacy external", raw: "authorization:\n  mode: dryRun\n  tlsProvider: external\n", want: "TLS provider transition"},
		{name: "legacy Off", raw: "server:\n  port: 9443\n"},
		{name: "legacy Off to external", raw: "server:\n  port: 9443\n"},
		{name: "legacy null", raw: "authorization: null\n", want: "legacy ucp-config"},
		{name: "legacy invalid", raw: "authorization: [", want: "legacy ucp-config"},
		{name: "legacy absent", want: "legacy ucp-config"},
		{name: "invalid record", record: "unknown", want: "invalid provider record"},
		{name: "foreign release", record: "radius", want: "foreign ownership"},
		{name: "foreign namespace", record: "radius", want: "foreign ownership"},
		{name: "terminating", record: "radius", want: "foreign ownership"},
		{name: "missing identity", record: "radius", want: "incomplete identity"},
		{name: "missing config", want: "ucp-config is missing"},
		{name: "forbidden lookup", record: "radius", want: "forbidden"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCertificateChartFixture(t)
			f.upgrade = true
			config := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "v1", "kind": "ConfigMap",
				"metadata": map[string]any{"name": "ucp-config", "namespace": RadiusSystemNamespace,
					"uid": "config-uid", "resourceVersion": "1",
					"labels":      map[string]any{"app.kubernetes.io/managed-by": "Helm"},
					"annotations": map[string]any{"meta.helm.sh/release-name": radiusReleaseName, "meta.helm.sh/release-namespace": RadiusSystemNamespace}},
				"data": map[string]any{"ucp-config.yaml": tc.raw},
			}}
			annotations := config.GetAnnotations()
			if tc.record != "" {
				annotations["radius.io/tls-provider"] = tc.record
			}
			switch tc.name {
			case "legacy Off to external":
				setValue(f.values, "external", "global", "rbac", "tls", "provider")
			case "foreign release":
				annotations["meta.helm.sh/release-name"] = "another"
			case "foreign namespace":
				annotations["meta.helm.sh/release-namespace"] = "another"
			case "terminating":
				require.NoError(t, unstructured.SetNestedField(config.Object, "2026-11-23T00:00:00Z", "metadata", "deletionTimestamp"))
			case "missing identity":
				config.SetUID("")
			case "missing config":
				config.SetName("unrelated")
			case "forbidden lookup":
				f.forbiddenPath = "/api/v1/namespaces/radius-system/configmaps/ucp-config"
			}
			config.SetAnnotations(annotations)
			f.objects = append(f.objects, config)
			rendered, err := f.renderChart(t)
			if tc.want != "" {
				require.ErrorContains(t, err, tc.want)
				require.Empty(t, rendered)
			} else {
				require.NoError(t, err)
			}
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
