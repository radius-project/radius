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
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/authz/mtls"
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
	fixture := newIssuerPolicyFixture(t)
	fixture.object("Namespace", RadiusSystemNamespace).SetAnnotations(map[string]string{
		bootstrapNamespaceIntent: "unresolved-intent",
	})
	selected, err := loader.Load("../../../deploy/Chart")
	require.NoError(t, err)
	server := fixture.server(t)
	renderer := engine.New(&rest.Config{Host: server.URL})
	_, err = renderer.Render(selected, common.Values{"Values": selected.Values, "Chart": selected.Metadata,
		"Capabilities": common.DefaultCapabilities,
		"Release":      map[string]any{"Name": radiusReleaseName, "Namespace": RadiusSystemNamespace, "Service": "Helm", "IsInstall": true}})
	require.ErrorContains(t, err, "bootstrap intent without its journal")
}

func TestBootstrapSelectedChart(t *testing.T) {
	for _, mode := range []string{"selfSigned", "caSecret", "issuerRef"} {
		for _, external := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/external=%t", mode, external), func(t *testing.T) {
				s, state := bootstrapStoreFixture(t)
				fixture := newIssuerPolicyFixture(t)
				selected, err := loader.Load("../../../deploy/Chart")
				require.NoError(t, err)
				state.Mode, state.Phase, state.ProofPort = mode, bootstrapInstalling, 9090
				state.CA, _, _ = testSigningCA(t, nil, nil, nil, nil)
				state.CAPinnedAt = s.now().UTC()
				state.Signer, state.SignerUID = "operator-ca", "signer-uid"
				if mode == "selfSigned" {
					state.RootSecretUID = "root-secret-uid"
				}

				if external {
					state.Scope = rotationAggregationOnly
				}
				if mode == "issuerRef" {
					_, issuerState, policy := rotationIssuerFixture(t, external, false)
					fixture = policy
					state.CA, state.External = issuerState.NewCA, issuerState.Plan.ExternalIssuer
					state.Signer, state.SignerUID = "", ""
				}
				state.TrustSHA256, _, err = mtls.TrustBundleDigest(state.CA, s.now())
				require.NoError(t, err)
				values := copyValues(fixture.values)
				provider := "radius"
				if external {
					provider = "external"
				}
				require.NoError(t, unstructured.SetNestedField(values, provider, "global", "rbac", "mtls", "provider"))
				require.NoError(t, unstructured.SetNestedField(values, !external, "de", "ucpMTLS", "enabled"))
				if external {
					values["de"] = externalProviderValues()["de"]
				}
				require.NoError(t, unstructured.SetNestedField(values, true, "global", "prometheus", "enabled"))
				require.NoError(t, unstructured.SetNestedField(values, int64(state.ProofPort), "global", "prometheus", "port"))
				settings := map[string]any{"mode": mode}
				if mode == "caSecret" {
					settings["caSecret"], settings["caBundle"] = state.Signer, string(state.CA)
				} else if mode == "issuerRef" {
					settings = certificateValues(values)
					settings["caBundle"] = string(state.CA)
				}
				setCertificateValues(values, settings)
				values, err = valuesutil.CoalesceValues(selected, values)
				require.NoError(t, err)
				installer := &aggregationBootstrapInstaller{chart: selected, values: values}
				values, err = installer.installValues(state)
				require.NoError(t, err)
				state.InstallValuesSHA256, err = bootstrapValuesDigest(values)
				require.NoError(t, err)
				require.NoError(t, state.validate(s.now()))
				require.NoError(t, unstructured.SetNestedField(values, "lease-uid", "global", "rbac", bootstrapInvocationValue))
				raw, err := json.Marshal(state)
				require.NoError(t, err)
				journal := &unstructured.Unstructured{Object: map[string]any{
					"apiVersion": "v1", "kind": "ConfigMap",
					"metadata": map[string]any{"name": aggregationBootstrapName, "namespace": s.namespace,
						"uid": "bootstrap-uid", "resourceVersion": "1", "labels": map[string]any{aggregationBootstrapOwner: s.release}},
					"data": map[string]any{rotationJournalKey: string(raw)},
				}}
				fixture.objects = append(fixture.objects, journal, &unstructured.Unstructured{Object: map[string]any{
					"apiVersion": "coordination.k8s.io/v1", "kind": "Lease",
					"metadata": map[string]any{"name": identityOperationLeaseName, "namespace": s.namespace, "uid": "lease-uid", "resourceVersion": "1"},
				}})
				server := fixture.server(t)
				renderer := engine.New(&rest.Config{Host: server.URL})
				render := func() (map[string]string, error) {
					return renderer.Render(selected, common.Values{"Values": values, "Chart": selected.Metadata,
						"Capabilities": common.DefaultCapabilities,
						"Release":      map[string]any{"Name": s.release, "Namespace": s.namespace, "Service": "Helm", "IsInstall": true}})
				}
				rendered, err := render()
				require.NoError(t, err)
				require.NotContains(t, rendered["radius/templates/ucp/apiservice.yaml"], "kind: Secret")
				require.NotContains(t, rendered["radius/templates/ucp/apiservice.yaml"], "ucp-ca")
				require.Contains(t, rendered["radius/templates/ucp/apiservice.yaml"], aggregationBootstrapOwner)
				require.Contains(t, rendered["radius/templates/ucp/deployment.yaml"], "UCP_AGGREGATION_STARTUP_PROOF_ENABLED")
				require.Contains(t, rendered["radius/templates/ucp/deployment.yaml"], "ucp-cert-aggregation-"+strings.ReplaceAll(state.ID, "-", "")[:16])
				certificates := rendered["radius/templates/global/certificates.yaml"]
				require.Equal(t, !external, strings.Contains(certificates, "name: ucp-cert\n"))
				unstructured.RemoveNestedField(values, "global", "rbac", bootstrapInvocationValue)
				_, err = render()
				require.ErrorContains(t, err, "matching leased Radius install")
				require.NoError(t, unstructured.SetNestedField(values, "lease-uid", "global", "rbac", bootstrapInvocationValue))
				require.NoError(t, unstructured.SetNestedField(values, "different-image", "ucp", "image"))
				_, err = render()
				require.ErrorContains(t, err, "install settings")
			})
		}
	}
}
