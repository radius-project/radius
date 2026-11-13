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
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
)

func TestTokenCertificateModesChart(t *testing.T) {
	for _, mode := range []string{"selfSigned", "caSecret", "issuerRef"} {
		for _, provider := range []string{"radius", "external"} {
			for _, enforcement := range []string{"off", "dryRun", "enforce"} {
				t.Run(mode+"/"+provider+"/"+enforcement, func(t *testing.T) {
					f := newIssuerPolicyFixture(t)
					rbac := f.values["global"].(map[string]any)["rbac"].(map[string]any)
					rbac["enabled"], rbac["dryRun"] = enforcement == "enforce", enforcement == "dryRun"
					rbac["tls"] = map[string]any{"provider": provider}
					if provider == "external" {
						var objects []*unstructured.Unstructured
						for _, obj := range f.objects {
							if !strings.HasPrefix(obj.GetName(), "cert-manager-csi-driver-spiffe-approver") && obj.GetName() != "spiffe-approver" {
								objects = append(objects, obj)
							}
						}
						f.objects = objects
					}
					if mode != "issuerRef" {
						c := map[string]any{"mode": mode}
						if mode == "caSecret" {
							c["caSecret"] = "operator-signing-ca"
							c["caBundle"] = "-----BEGIN CERTIFICATE-----\ndGVzdA==\n-----END CERTIFICATE-----\n"
						}
						rbac["certificates"] = c
					}
					rendered, err := f.renderChart(t)
					require.NoError(t, err)
					objects := chartObjects(t, rendered)
					find := func(kind, name string) *unstructured.Unstructured {
						for _, obj := range objects {
							if obj.GetKind() == kind && obj.GetName() == name {
								return obj
							}
						}
						return nil
					}
					if enforcement == "off" {
						require.Nil(t, find("Issuer", "radius-ca-issuer"))
						require.Nil(t, find("Secret", "radius-apiserver-trust"))
						require.Nil(t, find("ConfigMap", "radius-mtls-ca"))
						return
					}
					api := find("APIService", "v1alpha3.api.ucp.dev")
					require.Equal(t, "radius-system/radius-apiserver-trust", api.GetAnnotations()[injectCAFromSecretAnnotation])
					require.Nil(t, find("Secret", "operator-signing-ca"))
					aggregation := find("Certificate", "ucp-cert")
					require.NotNil(t, aggregation)
					dns, _, err := unstructured.NestedStringSlice(aggregation.Object, "spec", "dnsNames")
					require.NoError(t, err)
					require.Equal(t, []string{"ucp.radius-system.svc"}, dns)
					for _, component := range []string{"ucp", "applications-rp", "dynamic-rp", "controller", "bicep-de"} {
						require.Nil(t, find("Secret", component+"-mtls"))
						if component != "ucp" {
							require.Nil(t, find("Certificate", component+"-cert"))
						}
					}
					if mode == "selfSigned" {
						require.Nil(t, find("Secret", "radius-apiserver-trust"), "a second connected render follows CA issuance")
						require.NotNil(t, find("Certificate", "radius-apiserver-ca"))
					} else {
						trust := find("Secret", "radius-apiserver-trust")
						require.NotNil(t, trust)
						require.Equal(t, "true", trust.GetAnnotations()[allowDirectInjectionAnnotation])
						data, _, err := unstructured.NestedMap(trust.Object, "data")
						require.NoError(t, err)
						require.Len(t, data, 1)
						require.Nil(t, find("Certificate", "radius-apiserver-ca"))
					}
					if mode == "caSecret" {
						for _, issuerName := range []string{"radius-apiserver-issuer", "radius-ca-issuer"} {
							if issuerName == "radius-ca-issuer" && provider == "external" {
								continue
							}
							name, _, err := unstructured.NestedString(find("Issuer", issuerName).Object, "spec", "ca", "secretName")
							require.NoError(t, err)
							require.Equal(t, "operator-signing-ca", name)
						}
					}
					if provider == "external" {
						require.Nil(t, find("Issuer", "radius-ca-issuer"))
						require.Nil(t, find("Role", "radius-spiffe-certificaterequests"))
						require.Nil(t, find("ConfigMap", "radius-mtls-ca"))
					}
					for _, component := range []string{"ucp", "applications-rp", "dynamic-rp", "controller"} {
						volumes, _, err := unstructured.NestedSlice(find("Deployment", component).Object, "spec", "template", "spec", "volumes")
						require.NoError(t, err)
						found := false
						for _, raw := range volumes {
							volume := raw.(map[string]any)
							if volume["name"] != "radius-ca" {
								continue
							}
							found = true
							source := volume["configMap"]
							if mode == "selfSigned" {
								source = volume["secret"]
							}
							require.Equal(t, []any{map[string]any{"key": "ca.crt", "path": "ca.crt"}}, source.(map[string]any)["items"])
						}
						require.Equal(t, provider == "radius", found)
					}
				})
			}
		}
	}
}

func chartObjects(t *testing.T, rendered map[string]string) []*unstructured.Unstructured {
	t.Helper()
	var objects []*unstructured.Unstructured
	for name, text := range rendered {
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}
		decoder := yamlutil.NewYAMLOrJSONDecoder(bytes.NewBufferString(text), 4096)
		for {
			var obj unstructured.Unstructured
			err := decoder.Decode(&obj)
			if err == io.EOF {
				break
			}
			require.NoError(t, err, name)
			if obj.Object != nil {
				objects = append(objects, &obj)
			}
		}
	}
	return objects
}

func TestCertificateReceiverInventory(t *testing.T) {
	for _, tt := range []struct {
		name, provider string
		de             bool
		expected       []string
	}{
		{name: "managed default", provider: "radius", expected: []string{"ucp", "applications-rp", "dynamic-rp"}},
		{name: "managed engine gate", provider: "radius", de: true, expected: []string{"ucp", "applications-rp", "dynamic-rp", "bicep-de"}},
		{name: "external engine gate", provider: "external", de: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newIssuerPolicyFixture(t)
			rbac := f.values["global"].(map[string]any)["rbac"].(map[string]any)
			rbac["certificates"] = map[string]any{"mode": "selfSigned"}
			rbac["tls"] = map[string]any{"provider": tt.provider}
			f.values["de"] = map[string]any{"internalAuth": map[string]any{"enabled": tt.de}}
			rendered, err := f.renderChart(t)
			require.NoError(t, err)
			var actual []string
			for _, obj := range chartObjects(t, rendered) {
				if obj.GetKind() != "RoleBinding" || obj.GetName() != "radius-spiffe-certificaterequests" {
					continue
				}
				subjects, _, err := unstructured.NestedSlice(obj.Object, "subjects")
				require.NoError(t, err)
				for _, subject := range subjects {
					actual = append(actual, subject.(map[string]any)["name"].(string))
				}
			}
			require.ElementsMatch(t, tt.expected, actual)
		})
	}
}

func TestCertificatePublicSourceGuards(t *testing.T) {
	const selected = "-----BEGIN CERTIFICATE-----\ndGVzdA==\n-----END CERTIFICATE-----\n"
	for _, tt := range []struct {
		name   string
		change func(*issuerPolicyFixture)
		object *unstructured.Unstructured
		err    string
	}{
		{name: "missing signing name", change: func(f *issuerPolicyFixture) {
			f.values["global"].(map[string]any)["rbac"].(map[string]any)["certificates"].(map[string]any)["caSecret"] = ""
		}, err: "existing operator-owned TLS Secret"},
		{name: "managed signing name", change: func(f *issuerPolicyFixture) {
			f.values["global"].(map[string]any)["rbac"].(map[string]any)["certificates"].(map[string]any)["caSecret"] = "radius-apiserver-ca"
		}, err: "must not name a Radius-managed"},
		{name: "invalid signing name", change: func(f *issuerPolicyFixture) {
			f.values["global"].(map[string]any)["rbac"].(map[string]any)["certificates"].(map[string]any)["caSecret"] = "invalid/name"
		}, err: "existing operator-owned TLS Secret"},
		{name: "unfinished or unknown journal", object: &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "radius-ca-rotation", "namespace": "radius-system"},
			"data": map[string]any{"state.json": "{}"},
		}}, err: "CA lifecycle journal has foreign or incomplete ownership"},
		{name: "public source carries extra data", object: &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "radius-mtls-ca", "namespace": "radius-system"},
			"data": map[string]any{"ca.crt": selected, "tls.key": "must not be copied"},
		}}, err: "contains other data"},
		{name: "different live public source", object: &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "radius-mtls-ca", "namespace": "radius-system"},
			"data": map[string]any{"ca.crt": "different CA"},
		}}, err: "differs from the selected CA"},
		{name: "mode switch", object: &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "radius-mtls-ca", "namespace": "radius-system",
				"annotations": map[string]any{"radius.io/certificate-mode": "selfSigned"}},
			"data": map[string]any{"ca.crt": selected},
		}}, err: "belongs to another certificate mode"},
		{name: "retargeting an issuer", object: &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "cert-manager.io/v1", "kind": "Issuer", "metadata": map[string]any{"name": "radius-ca-issuer", "namespace": "radius-system"},
			"spec": map[string]any{"ca": map[string]any{"secretName": "previous-signing-secret"}},
		}}, err: "would be removed or retargeted"},
		{name: "same live public source preserves bytes", object: &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "radius-mtls-ca", "namespace": "radius-system"},
			"data": map[string]any{"ca.crt": "\n" + selected + "\n"},
		}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newIssuerPolicyFixture(t)
			f.values["global"].(map[string]any)["rbac"].(map[string]any)["certificates"] = map[string]any{
				"mode": "caSecret", "caSecret": "operator-ca", "caBundle": selected,
			}

			if tt.change != nil {
				tt.change(f)
			}
			if tt.object != nil {
				f.objects = append(f.objects, tt.object)
			}
			rendered, err := f.renderChart(t)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			for _, obj := range chartObjects(t, rendered) {
				if obj.GetKind() == "ConfigMap" && obj.GetName() == "radius-mtls-ca" {
					value, _, err := unstructured.NestedString(obj.Object, "data", "ca.crt")
					require.NoError(t, err)
					require.Equal(t, "\n"+selected+"\n", value)
				}
			}
		})
	}
}
