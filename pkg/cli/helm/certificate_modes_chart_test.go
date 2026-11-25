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
	"encoding/base64"
	"encoding/json"
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
					f := newCertificateChartFixture(t)
					rbac := f.values["global"].(map[string]any)["rbac"].(map[string]any)
					rbac["enabled"], rbac["dryRun"] = enforcement == "enforce", enforcement == "dryRun"
					rbac["tls"] = map[string]any{"provider": provider}

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
					for _, object := range objects {
						if object.GetKind() != "Deployment" {
							continue
						}
						volumes, _, err := unstructured.NestedSlice(object.Object, "spec", "template", "spec", "volumes")
						require.NoError(t, err)
						for _, raw := range volumes {
							secret, ok := raw.(map[string]any)["secret"].(map[string]any)
							if !ok {
								continue
							}
							switch secret["secretName"] {
							case "radius-apiserver-ca", "operator-signing-ca":
								t.Fatalf("%s mounts a signing Secret: %v", object.GetName(), secret)
							case "radius-ca":
								require.Equal(t, []any{map[string]any{"key": "ca.crt", "path": "ca.crt"}}, secret["items"])
							}
						}
					}
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
					sourceName := "radius-apiserver-trust"
					if mode == "selfSigned" {
						sourceName = "radius-apiserver-ca"
					}
					require.Equal(t, "radius-system/"+sourceName, api.GetAnnotations()[injectCAFromSecretAnnotation])
					require.Nil(t, find("Secret", "operator-signing-ca"))
					aggregation := find("Certificate", "ucp-cert")
					require.NotNil(t, aggregation)
					dns, _, err := unstructured.NestedStringSlice(aggregation.Object, "spec", "dnsNames")
					require.NoError(t, err)
					require.Equal(t, []string{"ucp.radius-system.svc"}, dns)
					for _, receiver := range []string{"ucp", "applications-rp", "dynamic-rp"} {
						leaf := find("Certificate", receiver+"-spiffe")
						if provider == "external" {
							require.Nil(t, leaf)
							continue
						}
						require.NotNil(t, leaf)
						spec := leaf.Object["spec"].(map[string]any)
						require.Equal(t, receiver+"-spiffe", spec["secretName"])
						require.Equal(t, []any{"spiffe://radius.internal/ns/radius-system/sa/" + receiver}, spec["uris"])
						require.NotContains(t, spec, "dnsNames")
						require.Equal(t, []any{"digital signature", "server auth"}, spec["usages"])
						name := "radius-ca-issuer"
						if mode == "issuerRef" {
							name = "external-ca"
						}
						require.Equal(t, name, spec["issuerRef"].(map[string]any)["name"])
					}
					for _, component := range []string{"ucp", "applications-rp", "dynamic-rp", "controller", "bicep-de"} {
						require.Nil(t, find("Secret", component+"-mtls"))
						if component != "ucp" {
							require.Nil(t, find("Certificate", component+"-cert"))
						}
					}
					if mode == "selfSigned" {
						require.Nil(t, find("Secret", "radius-apiserver-trust"), "selfSigned follows its owned signer directly")
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

func TestCertificateModesDoNotAddSecretReaderPermissions(t *testing.T) {
	grants := func(objects []*unstructured.Unstructured) []string {
		var result []string
		for _, object := range objects {
			if object.GetKind() != "Role" && object.GetKind() != "ClusterRole" {
				continue
			}
			rules, _, err := unstructured.NestedSlice(object.Object, "rules")
			require.NoError(t, err)
			for _, raw := range rules {
				rule := raw.(map[string]any)
				resources, _, err := unstructured.NestedStringSlice(rule, "resources")
				require.NoError(t, err)
				for _, resource := range resources {
					if resource == "secrets" || resource == "*" {
						encoded, err := json.Marshal(rule)
						require.NoError(t, err)
						result = append(result, object.GetKind()+"/"+object.GetName()+":"+string(encoded))
						break
					}
				}
			}
		}
		return result
	}
	for _, provider := range []string{"radius", "external"} {
		t.Run(provider, func(t *testing.T) {
			f := newCertificateChartFixture(t)
			rbac := f.values["global"].(map[string]any)["rbac"].(map[string]any)
			issuerCertificates := rbac["certificates"]
			rbac["tls"] = map[string]any{"provider": provider}
			rbac["enabled"], rbac["dryRun"] = false, false

			rendered, err := f.renderChart(t)
			require.NoError(t, err)
			baseline := grants(chartObjects(t, rendered))
			rbac["dryRun"] = true
			for _, mode := range []string{"selfSigned", "caSecret", "issuerRef"} {
				if mode != "issuerRef" {
					rbac["certificates"] = map[string]any{"mode": mode}
					if mode == "caSecret" {
						rbac["certificates"].(map[string]any)["caSecret"] = "operator-signing-ca"
						rbac["certificates"].(map[string]any)["caBundle"] = "-----BEGIN CERTIFICATE-----\ndGVzdA==\n-----END CERTIFICATE-----\n"
					}
				} else {
					rbac["certificates"] = issuerCertificates
				}
				rendered, err := f.renderChart(t)
				require.NoError(t, err)
				require.ElementsMatch(t, baseline, grants(chartObjects(t, rendered)), mode+" must not add Secret readers")
			}
		})
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
			f := newCertificateChartFixture(t)
			rbac := f.values["global"].(map[string]any)["rbac"].(map[string]any)
			rbac["certificates"] = map[string]any{"mode": "selfSigned"}
			rbac["tls"] = map[string]any{"provider": tt.provider}
			f.values["de"] = map[string]any{"tag": "test-compatible", "internalAuth": map[string]any{"enabled": tt.de}}
			rendered, err := f.renderChart(t)
			require.NoError(t, err)
			var actual []string
			for _, obj := range chartObjects(t, rendered) {
				if obj.GetKind() == "Certificate" && strings.HasSuffix(obj.GetName(), "-spiffe") {
					actual = append(actual, strings.TrimSuffix(obj.GetName(), "-spiffe"))
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
		change func(*certificateChartFixture)
		object *unstructured.Unstructured
		err    string
	}{
		{name: "missing signing name", change: func(f *certificateChartFixture) {
			f.values["global"].(map[string]any)["rbac"].(map[string]any)["certificates"].(map[string]any)["caSecret"] = ""
		}, err: "existing operator-owned TLS Secret"},
		{name: "managed signing name", change: func(f *certificateChartFixture) {
			f.values["global"].(map[string]any)["rbac"].(map[string]any)["certificates"].(map[string]any)["caSecret"] = "radius-apiserver-ca"
		}, err: "must not name a Radius-managed"},
		{name: "shared root CA uses signing Secret", change: func(f *certificateChartFixture) {
			f.values["global"].(map[string]any)["rootCA"] = map[string]any{"cert": "public-ca", "secretName": "operator-ca"}
		}, err: "that Secret holds the operator's CA signing key"},
		{name: "external provider mounts signing Secret", change: func(f *certificateChartFixture) {
			global := f.values["global"].(map[string]any)
			global["rbac"].(map[string]any)["tls"] = map[string]any{"provider": "external"}
			global["targetCluster"] = map[string]any{"enabled": true, "secretName": "operator-ca"}
		}, err: "that Secret holds the operator's CA signing key"},
		{name: "invalid signing name", change: func(f *certificateChartFixture) {
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
			f := newCertificateChartFixture(t)
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

func TestSelfSignedInternalSource(t *testing.T) {
	for _, matching := range []bool{false, true} {
		f := newCertificateChartFixture(t)
		nestedValue(f.values, "global", "rbac").(map[string]any)["certificates"] = map[string]any{"mode": "selfSigned"}
		signer := newTestCA(t, "radius-ca").pem
		copy := signer
		if !matching {
			copy = newTestCA(t, "previous").pem
		}
		for _, obj := range []map[string]any{
			{"kind": "Secret", "type": "kubernetes.io/tls", "metadata": map[string]any{"name": "radius-ca", "annotations": map[string]any{
				certificateNameAnnotation: "radius-ca", "cert-manager.io/issuer-name": radiusBootstrapIssuerName,
				"cert-manager.io/issuer-kind": "Issuer", "cert-manager.io/issuer-group": "cert-manager.io",
			}}, "data": map[string]any{"ca.crt": base64.StdEncoding.EncodeToString(signer), "tls.crt": base64.StdEncoding.EncodeToString(signer), "tls.key": "fixture"}},
			{"kind": "ConfigMap", "metadata": map[string]any{"name": "radius-mtls-ca"}, "data": map[string]any{"ca.crt": string(copy)}},
		} {
			obj["apiVersion"] = "v1"
			o := &unstructured.Unstructured{Object: obj}
			o.SetNamespace(RadiusSystemNamespace)
			f.objects = append(f.objects, o)
		}
		rendered, err := f.renderChart(t)
		if !matching {
			require.ErrorContains(t, err, "differs from the selected CA")
			continue
		}
		require.NoError(t, err)
		found := false
		for _, obj := range chartObjects(t, rendered) {
			if obj.GetKind() != "Deployment" || obj.GetName() != "controller" {
				continue
			}
			for _, v := range nestedValue(obj.Object, "spec", "template", "spec", "volumes").([]any) {
				vol := v.(map[string]any)
				if vol["name"] == "radius-ca" {
					require.Equal(t, "radius-ca", nestedValue(vol, "secret", "secretName"))
					require.Equal(t, []any{map[string]any{"key": "ca.crt", "path": "ca.crt"}}, nestedValue(vol, "secret", "items"))
					found = true
				}
			}
		}
		require.True(t, found)
	}
}

func TestIssuerRefContinuity(t *testing.T) {
	// Kept trust survives removal of the issuing Certificates on uninstall.
	f := newCertificateChartFixture(t)
	rendered, err := f.renderChart(t)
	require.NoError(t, err)
	for _, obj := range chartObjects(t, rendered) {
		if obj.GetKind() == "Secret" && obj.GetName() == ucpAPIServerTrustSecretName {
			f.objects = append(f.objects, obj)
		}
	}
	issuer := f.objects[1].DeepCopy()
	issuer.SetName("next-ca")
	f.objects = append(f.objects, issuer)
	nestedValue(f.values, "global", "rbac", "certificates", "issuerRef").(map[string]any)["name"] = "next-ca"
	_, err = f.renderChart(t)
	require.ErrorContains(t, err, "retained aggregation trust issuerRef differs")

	for _, prior := range []string{"external-ca", "previous-ca"} {
		f := newCertificateChartFixture(t)
		f.upgrade = true
		f.objects = append(f.objects, &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "v1", "kind": "ConfigMap",
			"metadata": map[string]any{"namespace": RadiusSystemNamespace, "name": "ucp-config",
				"uid": "config-uid", "resourceVersion": "1",
				"labels": map[string]any{helmManagedByLabel: helmManagedByValue},
				"annotations": map[string]any{helmReleaseNameAnnotation: radiusReleaseName,
					helmReleaseNamespaceAnnotation: RadiusSystemNamespace, "radius.io/tls-provider": "radius"}},
		}})
		f.objects = append(f.objects, &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "cert-manager.io/v1", "kind": "Certificate",
			"metadata": map[string]any{"namespace": RadiusSystemNamespace, "name": "ucp-cert"},
			"spec":     map[string]any{"issuerRef": map[string]any{"group": "cert-manager.io", "kind": "Issuer", "name": prior}},
		}})
		_, err := f.renderChart(t)
		if prior == "external-ca" {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "issuerRef differs")
		}
	}
}

func TestBYOAggregationSecretNames(t *testing.T) {
	for _, mode := range []string{"caSecret", "issuerRef"} {
		for _, shared := range []string{"rootCA", "targetCluster", "imagePullSecrets", "distinct"} {
			t.Run(mode+"/"+shared, func(t *testing.T) {
				f := newCertificateChartFixture(t)
				global := f.values["global"].(map[string]any)
				if mode == "caSecret" {
					c := nestedValue(global, "rbac", "certificates").(map[string]any)
					delete(c, "issuerRef")
					c["mode"], c["caSecret"] = mode, "operator-ca"
				}
				root := map[string]any{"cert": "outbound-ca", "secretName": "outbound-trust"}
				target := map[string]any{"enabled": true, "secretName": "target-kubeconfig"}
				global["rootCA"], global["targetCluster"] = root, target
				field := "global." + shared + ".secretName"
				if shared == "imagePullSecrets" {
					global[shared] = []any{map[string]any{"name": ucpAPIServerTrustSecretName}}
					field = "global.imagePullSecrets[0].name"
				} else if shared != "distinct" {
					global[shared].(map[string]any)["secretName"] = ucpAPIServerTrustSecretName
				}
				rendered, err := f.renderChart(t)
				if shared != "distinct" {
					require.ErrorContains(t, err, field+` must not be "radius-apiserver-trust"`)
					return
				}
				require.NoError(t, err)
				var sources int
				for _, obj := range chartObjects(t, rendered) {
					if obj.GetKind() == "Secret" && obj.GetName() == ucpAPIServerTrustSecretName {
						sources++
					}
				}
				require.Equal(t, 1, sources)
			})
		}
	}
}

func TestOffCertificateJournal(t *testing.T) {
	f := newCertificateChartFixture(t)
	f.upgrade = true
	f.objects = append(f.objects, &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]any{"namespace": RadiusSystemNamespace, "name": "ucp-config",
			"uid": "config-uid", "resourceVersion": "1",
			"labels": map[string]any{helmManagedByLabel: helmManagedByValue},
			"annotations": map[string]any{helmReleaseNameAnnotation: radiusReleaseName,
				helmReleaseNamespaceAnnotation: RadiusSystemNamespace, "radius.io/tls-provider": "radius"}},
	}})
	nestedValue(f.values, "global", "rbac").(map[string]any)["enabled"] = false
	_, err := f.renderChart(t)
	require.NoError(t, err)
	f.objects = append(f.objects, &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]any{"name": "radius-ca-rotation", "namespace": RadiusSystemNamespace},
	}})
	_, err = f.renderChart(t)
	require.ErrorContains(t, err, "radius-ca-rotation exists")
}
