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
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/chart/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func reissueTestCA(t *testing.T, ca testCA, notAfter time.Time) testCA {
	t.Helper()
	template := *ca.cert
	template.SerialNumber = big.NewInt(time.Now().UnixNano())
	template.NotBefore, template.NotAfter = time.Now().Add(-time.Minute), notAfter
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &ca.key.PublicKey, ca.key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return testCA{cert: cert, key: ca.key, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

func mustParseCertificate(t *testing.T, pemBytes []byte) *x509.Certificate {
	t.Helper()
	cert, err := firstCertificate(pemBytes)
	require.NoError(t, err)
	return cert
}

func retainedAggregationTrust(ca testCA) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: ucpAPIServerTrustSecretName, Namespace: RadiusSystemNamespace, UID: "retained-copy", ResourceVersion: "7",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "Helm"},
			Annotations: map[string]string{
				"radius.io/certificate-mode": "selfSigned", allowDirectInjectionAnnotation: "true",
				"meta.helm.sh/release-name": "radius", "meta.helm.sh/release-namespace": RadiusSystemNamespace,
				"helm.sh/resource-policy": "keep",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{caCertificateKey: ca.pem},
	}
}

func TestSelfSignedAggregationRenewalGuards(t *testing.T) {
	old := reissueTestCA(t, newTestCA(t, ucpAPIServerCASecretName), time.Now().Add(time.Minute))
	renewed := reissueTestCA(t, old, time.Now().Add(2*time.Hour))
	require.Equal(t, old.cert.RawSubjectPublicKeyInfo, renewed.cert.RawSubjectPublicKeyInfo)
	require.Equal(t, old.cert.RawSubject, renewed.cert.RawSubject)
	require.NotEqual(t, old.cert.SerialNumber, renewed.cert.SerialNumber)
	require.True(t, renewed.cert.NotAfter.After(old.cert.NotAfter))

	subjectTemplate := *renewed.cert
	subjectTemplate.RawSubject = nil
	subjectTemplate.Subject.Organization = []string{"unexpected"}
	subjectTemplate.SerialNumber = big.NewInt(123456)
	der, err := x509.CreateCertificate(rand.Reader, &subjectTemplate, &subjectTemplate, &old.key.PublicKey, old.key)
	require.NoError(t, err)
	changedSubject, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	for _, tt := range []struct {
		name string
		ca   testCA
		edit func(*corev1.Secret, *corev1.Secret)
		want string
	}{
		{name: "same key and subject renewal", ca: renewed},
		{name: "different key", ca: newTestCA(t, ucpAPIServerCASecretName), want: "key or subject changed"},
		{name: "different subject with same key", ca: testCA{cert: changedSubject, key: old.key,
			pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}, want: "key or subject changed"},
		{name: "foreign signer", ca: renewed, edit: func(s, _ *corev1.Secret) {
			s.Annotations[certificateNameAnnotation] = "operator-certificate"
		}, want: "foreign or has an invalid profile"},
		{name: "unexpected issuer", ca: renewed, edit: func(s, _ *corev1.Secret) {
			s.Annotations["cert-manager.io/issuer-name"] = "operator-issuer"
		}, want: "foreign or has an invalid profile"},
		{name: "foreign retained copy", ca: renewed, edit: func(_, pin *corev1.Secret) {
			pin.Labels["app.kubernetes.io/managed-by"] = "operator"
		}, want: "foreign or invalid"},
		{name: "invalid retained copy type", ca: renewed, edit: func(_, pin *corev1.Secret) {
			pin.Type = corev1.SecretTypeTLS
		}, want: "foreign or invalid"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			signer, pin := apiServerCASecret(tt.ca), retainedAggregationTrust(old)
			if tt.edit != nil {
				tt.edit(signer, pin)
			}
			client := kubefake.NewSimpleClientset(signer, pin)
			cert, bundle, problem, err := readUCPAPIServerCA(t.Context(), client, RadiusSystemNamespace)
			if tt.want != "" {
				require.ErrorContains(t, err, tt.want)
				require.Nil(t, cert)
				require.Nil(t, bundle)
			} else {
				require.NoError(t, err)
				require.Empty(t, problem)
				require.Equal(t, renewed.cert.Raw, cert.Raw)
			}
			for _, action := range client.Actions() {
				require.Equal(t, "get", action.GetVerb(), "preflight must not alter either Secret")
			}
			if tt.edit == nil && tt.want != "" {
				// Without the retained copy, the APIService's existing root
				// must still pin identity before either lifecycle write path.
				client := kubefake.NewSimpleClientset(signer)
				api := ucpAPIService(base64.StdEncoding.EncodeToString(old.pem), "True", "Passed")
				dynamic := ucpAPIServiceDynamicClient(api)
				impl := trustImpl(client, dynamic)
				render := func(context.Context) (string, bool, error) {
					t.Fatal("identity must be checked before Helm render or mutation")
					return "", false, nil
				}
				require.ErrorContains(t, impl.stageUCPAPIServerCA(t.Context(), "test", RadiusSystemNamespace,
					"radius", deployedMode(authz.ModeDryRun), authz.ModeDryRun, render), tt.want)
				require.ErrorContains(t, impl.reconcileUCPAPIServerTrust(t.Context(), "test", RadiusSystemNamespace), tt.want)
				got, err := dynamic.Resource(apiServicesResource).Get(t.Context(), ucpAPIServiceName, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, api, got, "failed continuity checks must not alter APIService trust")
			}
		})
	}
}

func TestSelfSignedAggregationSourceAndRenewal(t *testing.T) {
	old := reissueTestCA(t, newTestCA(t, ucpAPIServerCASecretName), time.Now().Add(time.Minute))
	renewed := reissueTestCA(t, old, time.Now().Add(2*time.Hour))
	for _, provider := range []string{"radius", "external"} {
		t.Run(provider, func(t *testing.T) {
			f := newCertificateChartFixture(t)
			rbac := f.values["global"].(map[string]any)["rbac"].(map[string]any)
			rbac["certificates"] = map[string]any{"mode": "selfSigned"}
			rbac["tls"] = map[string]any{"provider": provider}
			signer := apiServerCASecret(old)
			signer.UID, signer.ResourceVersion = "stable-signer", "1"
			pin := retainedAggregationTrust(old)
			for _, object := range []*corev1.Secret{signer, pin} {
				data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
				require.NoError(t, err)
				data["apiVersion"], data["kind"] = "v1", "Secret"
				f.objects = append(f.objects, &unstructured.Unstructured{Object: data})
			}
			manifests, err := f.renderChart(t, &common.File{Name: "templates/source-contract.json",
				Data: []byte(`{{ include "radius.authz.certificates.publicSource" (dict "root" . "plane" "aggregation") }}`)})
			require.NoError(t, err)
			var source struct {
				Kind, Name, Key, Bundle, UID, ResourceVersion, ContentDigest, IssuerDigest, Digest string
				Issuer                                                                             map[string]string
			}
			require.NoError(t, json.Unmarshal([]byte(manifests["radius/templates/source-contract.json"]), &source))
			require.Equal(t, "Secret", source.Kind)
			require.Equal(t, ucpAPIServerCASecretName, source.Name)
			require.Equal(t, caCertificateKey, source.Key)
			require.Equal(t, string(old.pem), source.Bundle)
			require.Equal(t, "stable-signer", source.UID)
			require.Equal(t, "1", source.ResourceVersion)
			require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(old.pem)), source.ContentDigest)
			require.NotEmpty(t, source.IssuerDigest)
			require.NotEmpty(t, source.Digest)
			require.Equal(t, radiusAPIServerIssuerName, source.Issuer["name"])
			delete(manifests, "radius/templates/source-contract.json")
			var api *unstructured.Unstructured
			for _, obj := range chartObjects(t, manifests) {
				require.False(t, obj.GetKind() == "Secret" && obj.GetName() == ucpAPIServerTrustSecretName,
					"the kept A8 copy must not be rewritten or adopted")
				if obj.GetKind() == "APIService" {
					api = obj
				}
			}
			require.NotNil(t, api)
			require.Equal(t, RadiusSystemNamespace+"/"+source.Name, api.GetAnnotations()[injectCAFromSecretAnnotation])

			// A read-only injection fixture follows the annotation to the renewed
			// Secret. No second Helm render or rad invocation publishes this CA.
			client := kubefake.NewSimpleClientset(signer, pin)
			live, err := client.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), source.Name, metav1.GetOptions{})
			require.NoError(t, err)
			live.Data[corev1.TLSCertKey], live.Data[caCertificateKey] = renewed.pem, renewed.pem
			_, err = client.CoreV1().Secrets(RadiusSystemNamespace).Update(t.Context(), live, metav1.UpdateOptions{})
			require.NoError(t, err)
			cluster := &stageCluster{kube: client}
			encoded, _, err := unstructured.NestedString(api.Object, "spec", "caBundle")
			require.NoError(t, err)
			bundle, err := base64.StdEncoding.DecodeString(encoded)
			require.NoError(t, err)
			bundle = cluster.cainjectorMerges(t, api, bundle)
			leaf := mustParseCertificate(t, issueUCPCertificate(t, renewed))
			future := old.cert.NotAfter.Add(time.Second)
			roots := x509.NewCertPool()
			require.True(t, roots.AppendCertsFromPEM(bundle))
			_, err = leaf.Verify(x509.VerifyOptions{Roots: roots, CurrentTime: future,
				DNSName: "ucp." + RadiusSystemNamespace + ".svc", KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
			require.NoError(t, err)
			frozen := x509.NewCertPool()
			frozen.AddCert(old.cert)
			_, err = leaf.Verify(x509.VerifyOptions{Roots: frozen, CurrentTime: future,
				DNSName: "ucp." + RadiusSystemNamespace + ".svc", KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
			require.Error(t, err, "the frozen A8 copy cannot verify after its root expires")
			var expired x509.CertificateInvalidError
			require.ErrorAs(t, err, &expired)
			require.Equal(t, x509.Expired, expired.Reason)
			got, err := client.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), source.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, types.UID("stable-signer"), got.UID)
			require.Equal(t, signer.Annotations, got.Annotations)
			kept, err := client.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), pin.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, pin, kept)
		})
	}
}

func TestSelfSignedAggregationStructuralGuards(t *testing.T) {
	for _, provider := range []string{"radius", "external"} {
		for _, field := range []string{"type", certificateNameAnnotation, "cert-manager.io/issuer-name",
			"cert-manager.io/issuer-kind", "cert-manager.io/issuer-group", "tls.crt", "tls.key", "ca.crt"} {
			t.Run(provider+"/"+field, func(t *testing.T) {
				f := newCertificateChartFixture(t)
				rbac := f.values["global"].(map[string]any)["rbac"].(map[string]any)
				rbac["certificates"] = map[string]any{"mode": "selfSigned"}
				rbac["tls"] = map[string]any{"provider": provider}
				signer := apiServerCASecret(newTestCA(t, ucpAPIServerCASecretName))
				switch {
				case field == "type":
					signer.Type = corev1.SecretTypeOpaque
				case strings.HasPrefix(field, "cert-manager.io/"):
					delete(signer.Annotations, field)
				default:
					delete(signer.Data, field)
				}
				data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(signer)
				require.NoError(t, err)
				data["apiVersion"], data["kind"] = "v1", "Secret"
				f.objects = append(f.objects, &unstructured.Unstructured{Object: data})
				_, err = f.renderChart(t)
				require.ErrorContains(t, err, "selfSigned aggregation prerequisite")
			})
		}
	}
}

func TestReconcileRetainsA8AggregationCopy(t *testing.T) {
	shortenUCPAPIServiceWait(t)
	old := newTestCA(t, ucpAPIServerCASecretName)
	renewed := reissueTestCA(t, old, time.Now().Add(2*time.Hour))
	pin := retainedAggregationTrust(old)
	signer := apiServerCASecret(renewed)
	signer.UID = "signer"
	client := kubefake.NewSimpleClientset(signer, pin)
	api := ucpAPIService(base64.StdEncoding.EncodeToString(old.pem), "True", "Passed")
	dynamic := ucpAPIServiceDynamicClient(api)
	require.NoError(t, trustImpl(client, dynamic).reconcileUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
	kept, err := client.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), pin.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, pin, kept)
	got, err := client.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), signer.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, signer, got)
	for _, action := range client.Actions() {
		require.Equal(t, "get", action.GetVerb())
	}
}

func TestRetainedAggregationTrustIsNotTransient(t *testing.T) {
	for _, mode := range []string{"selfSigned", "caSecret", "issuerRef", ""} {
		t.Run(mode, func(t *testing.T) {
			pin := retainedAggregationTrust(newTestCA(t, ucpAPIServerCASecretName))
			pin.Annotations["radius.io/certificate-mode"] = mode
			client := kubefake.NewSimpleClientset(pin)
			logs := []string{}
			impl := trustImpl(client, ucpAPIServiceDynamicClient())
			impl.logger = func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) }
			require.NoError(t, impl.deleteUCPAPIServerTrust(t.Context(), "test", RadiusSystemNamespace))
			require.Len(t, logs, 1)
			require.Contains(t, logs[0], "Keeping certificate lifecycle public trust")
			require.False(t, ownsUCPOffSecret(pin, ucpOffLeftoverSecrets[0]), "turning authorization off must not delete a lifecycle pin either")
			_, err := writeUCPAPIServerTrust(t.Context(), client, RadiusSystemNamespace, nil)
			require.ErrorContains(t, err, "preserve retained aggregation trust")
			kept, err := client.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), pin.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, pin, kept)
			for _, action := range client.Actions() {
				require.Equal(t, "get", action.GetVerb())
			}
		})
	}

}

func TestRetainedSelfSignedTrustReinstallAfterUninstall(t *testing.T) {
	for _, signerState := range []string{"kept", "deleted", "changed"} {
		t.Run(signerState, func(t *testing.T) {
			original := newTestCA(t, ucpAPIServerCASecretName)
			pin := retainedAggregationTrust(original)
			var objects []runtime.Object
			objects = append(objects, pin)
			switch signerState {
			case "kept":
				objects = append(objects, apiServerCASecret(original))
			case "changed":
				objects = append(objects, apiServerCASecret(newTestCA(t, ucpAPIServerCASecretName)))
			}
			client := kubefake.NewSimpleClientset(objects...)
			impl := trustImpl(client, ucpAPIServiceDynamicClient())
			impl.logger = func(string, ...any) {}
			require.NoError(t, impl.deleteUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
			err := impl.stageUCPAPIServerCA(t.Context(), "test-context", RadiusSystemNamespace, "radius", deployedMode(authz.ModeOff), authz.ModeEnforce, func(context.Context) (string, bool, error) {
				t.Fatal("no prior APIService remains; staging must not issue or replace a signer")
				return "", false, nil
			})
			switch signerState {
			case "kept":
				require.NoError(t, err, "the default kept signer must allow reinstall with its original key")
			case "deleted":
				require.ErrorContains(t, err, "exists without a usable radius-apiserver-ca signer")
				require.ErrorContains(t, err, `kubectl --context "test-context" -n radius-system apply -f radius-apiserver-ca-backup.yaml`)
				require.ErrorContains(t, err, `kubectl --context "test-context" -n radius-system delete secret radius-apiserver-trust`)
			case "changed":
				require.ErrorContains(t, err, "key or subject changed")
			}
			kept, err := client.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), pin.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, pin, kept)
			for _, action := range client.Actions() {
				require.Equal(t, "get", action.GetVerb())
			}
		})
	}
}

func TestBYOPublicTrustUninstallAndChartReinstall(t *testing.T) {
	for _, mode := range []string{"caSecret", "issuerRef"} {
		t.Run(mode, func(t *testing.T) {
			f := newCertificateChartFixture(t)
			rbac := f.values["global"].(map[string]any)["rbac"].(map[string]any)
			if mode == "caSecret" {
				rbac["certificates"] = map[string]any{"mode": mode, "caSecret": "operator-ca",
					"caBundle": "-----BEGIN CERTIFICATE-----\ndGVzdA==\n-----END CERTIFICATE-----\n"}
			}
			certs := rbac["certificates"].(map[string]any)
			pin := retainedAggregationTrust(testCA{pem: []byte(certs["caBundle"].(string))})
			pin.Annotations["radius.io/certificate-mode"] = mode
			client := kubefake.NewSimpleClientset(pin)
			impl := trustImpl(client, ucpAPIServiceDynamicClient())
			impl.logger = func(string, ...any) {}
			require.NoError(t, impl.deleteUCPAPIServerTrust(t.Context(), "test", RadiusSystemNamespace))
			kept, err := client.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), pin.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, pin, kept)
			data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(kept)
			require.NoError(t, err)
			data["apiVersion"], data["kind"] = "v1", "Secret"
			f.objects = append(f.objects, &unstructured.Unstructured{Object: data})
			_, err = f.renderChart(t)
			require.NoError(t, err, "chart reinstall can reuse exactly the pinned operator CA")
			certs["caBundle"] = "-----BEGIN CERTIFICATE-----\nb3RoZXI=\n-----END CERTIFICATE-----\n"
			_, err = f.renderChart(t)
			require.ErrorContains(t, err, "differs from the selected CA")
			require.ErrorContains(t, err, "use the lifecycle command")
			rbac["certificates"] = map[string]any{"mode": "selfSigned"}
			rbac["tls"] = map[string]any{"provider": "external"}
			_, err = f.renderChart(t)
			require.ErrorContains(t, err, "radius-apiserver-trust belongs to another certificate mode")
			err = impl.stageUCPAPIServerCA(t.Context(), "test", RadiusSystemNamespace, "radius", deployedMode(authz.ModeEnforce), authz.ModeEnforce, nil)
			require.ErrorContains(t, err, "use the certificate lifecycle before switching to selfSigned")
			for _, action := range client.Actions() {
				require.Equal(t, "get", action.GetVerb())
			}
		})
	}
}

func TestAggregationTrustSelectionGuards(t *testing.T) {
	for _, mode := range []string{"selfSigned", "caSecret", "issuerRef"} {
		for _, invalid := range []string{"foreign", "empty"} {
			t.Run(mode+"/"+invalid, func(t *testing.T) {
				f := newCertificateChartFixture(t)
				rbac := f.values["global"].(map[string]any)["rbac"].(map[string]any)
				if mode != "issuerRef" {
					rbac["certificates"] = map[string]any{"mode": mode}
					if mode == "caSecret" {
						rbac["certificates"] = map[string]any{"mode": mode, "caSecret": "operator-ca",
							"caBundle": "-----BEGIN CERTIFICATE-----\ndGVzdA==\n-----END CERTIFICATE-----\n"}
					}
				}
				copy := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: ucpAPIServerTrustSecretName, Namespace: RadiusSystemNamespace},
					Data: map[string][]byte{caCertificateKey: []byte("foreign"), "extra": []byte("do-not-trust")}}
				if invalid == "empty" {
					copy.Annotations = map[string]string{allowDirectInjectionAnnotation: "true"}
					copy.Data = map[string][]byte{caCertificateKey: {}}
				}
				for _, secret := range []*corev1.Secret{copy, apiServerCASecret(newTestCA(t, ucpAPIServerCASecretName))} {
					data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(secret)
					require.NoError(t, err)
					data["apiVersion"], data["kind"] = "v1", "Secret"
					f.objects = append(f.objects, &unstructured.Unstructured{Object: data})
				}
				rendered, err := f.renderChart(t)
				if mode != "selfSigned" {
					require.ErrorContains(t, err, "certificate lifecycle prerequisite")
					return
				}
				require.NoError(t, err)
				for _, object := range chartObjects(t, rendered) {
					if object.GetKind() == "APIService" {
						require.Equal(t, RadiusSystemNamespace+"/"+ucpAPIServerCASecretName,
							object.GetAnnotations()[injectCAFromSecretAnnotation], "an invalid transient source must not be used")
					}
				}
			})
		}
	}
}

func TestSelfSignedSwitchOnOverlapRender(t *testing.T) {
	for _, provider := range []string{"radius", "external"} {
		t.Run(provider, func(t *testing.T) {
			c := newStageCluster(t, nil, true)
			c.addOffAPIService(t, "off")
			impl := c.impl(nil)
			f := newCertificateChartFixture(t)
			rbac := f.values["global"].(map[string]any)["rbac"].(map[string]any)
			rbac["certificates"] = map[string]any{"mode": "selfSigned"}
			rbac["tls"] = map[string]any{"provider": provider}
			require.NoError(t, impl.stageUCPAPIServerCA(t.Context(), "test", RadiusSystemNamespace,
				"radius", deployedMode(authz.ModeOff), authz.ModeEnforce, func(context.Context) (string, bool, error) {
					return renderStageTestManifest(t, f.values), true, nil
				}))
			for _, name := range []string{ucpAPIServerTrustSecretName, ucpAPIServerCASecretName} {
				secret, err := c.kube.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), name, metav1.GetOptions{})
				require.NoError(t, err)
				if name == ucpAPIServerTrustSecretName {
					require.NotContains(t, secret.Annotations, "radius.io/certificate-mode")
				}
				data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(secret)
				require.NoError(t, err)
				data["apiVersion"], data["kind"] = "v1", "Secret"
				f.objects = append(f.objects, &unstructured.Unstructured{Object: data})
			}
			rendered, err := f.renderChart(t)
			require.NoError(t, err)
			found := false
			for _, object := range chartObjects(t, rendered) {
				if object.GetKind() != "APIService" {
					continue
				}
				found = true
				require.Equal(t, RadiusSystemNamespace+"/"+ucpAPIServerTrustSecretName, object.GetAnnotations()[injectCAFromSecretAnnotation])
				encoded, _, err := unstructured.NestedString(object.Object, "spec", "caBundle")
				require.NoError(t, err)
				bundle, err := base64.StdEncoding.DecodeString(encoded)
				require.NoError(t, err)
				roots := x509.NewCertPool()
				require.True(t, roots.AppendCertsFromPEM(bundle))
				for _, ca := range []testCA{c.ucpCA, c.apiServerCA} {
					leaf := mustParseCertificate(t, issueUCPCertificate(t, ca))
					_, err := leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: "ucp." + RadiusSystemNamespace + ".svc"})
					require.NoError(t, err, "Helm must trust both the still-running and new UCP certificate")
				}
			}
			require.True(t, found)
		})
	}
}

func TestBYOSwitchOnRequiresLifecycleBeforeStaging(t *testing.T) {
	for _, mode := range []string{"caSecret", "issuerRef"} {
		for _, provider := range []string{"radius", "external"} {
			for _, source := range []string{"flags", "stored", "rollback"} {
				t.Run(mode+"/"+provider+"/"+source, func(t *testing.T) {
					options := NewDefaultClusterOptions()
					options.Radius.SetArgs = []string{"global.rbac.enabled=true", "global.rbac.tls.provider=" + provider,
						"global.rbac.certificates.mode=" + mode}
					chart := loadStageTestChart(t)
					impl := &Impl{newUCPAPIServerTrustClient: func(string) (kubernetes.Interface, error) {
						t.Fatal("BYO switch-on must stop before connecting to the trust API")
						return nil, nil
					}}
					target := map[string]any{"global": map[string]any{"rbac": map[string]any{
						"enabled": true, "tls": map[string]any{"provider": provider}, "certificates": map[string]any{"mode": mode},
					}}}
					var err error
					switch source {
					case "flags", "stored":
						var stored map[string]any
						if source == "stored" {
							stored = target
							options.Radius.SetArgs = nil
						}
						prepared, parseErr := parseUserValuesFromCLI(&options.Radius)
						require.NoError(t, parseErr)
						err = impl.stageInstalledUCPAPIServerCA(t.Context(), options, "test", nil, chart.Values, stored, prepared, false, nil)
					case "rollback":
						history := []*releasev1.Release{{Version: 1, Chart: chart, Config: target}, {Version: 2, Chart: chart}}
						err = impl.stageRolledBackUCPAPIServerCA(t.Context(), "test", RadiusSystemNamespace, history, 1)
					}

					require.ErrorContains(t, err, "requires the certificate lifecycle")
					require.ErrorContains(t, err, "mode="+mode)
				})
			}
		}
	}
}

func TestRollbackCertificateModeBeforeStaging(t *testing.T) {
	for _, tt := range []struct {
		name, chartMode, override, refusedMode string
	}{
		{"chart CA secret", "caSecret", "", "caSecret"},
		{"chart issuer reference", "issuerRef", "", "issuerRef"},
		{"saved issuer overrides CA secret", "caSecret", "issuerRef", "issuerRef"},
		{"saved self-signed overrides CA secret", "caSecret", "selfSigned", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			target := loadStageTestChart(t)
			target.Values["global"].(map[string]any)["rbac"].(map[string]any)["certificates"] = map[string]any{"mode": tt.chartMode}
			rbac := map[string]any{"enabled": true}
			if tt.override != "" {
				rbac["certificates"] = map[string]any{"mode": tt.override}
			}
			history := []*releasev1.Release{
				{Version: 1, Chart: target, Config: map[string]any{"global": map[string]any{"rbac": rbac}}},
				{Version: 2, Chart: loadStageTestChart(t)},
			}
			called := false
			impl := newStageCluster(t, nil, false).impl(nil)
			impl.newUCPAPIServerTrustClient = func(string) (kubernetes.Interface, error) {
				called = true
				return nil, fmt.Errorf("stage attempted trust client")
			}
			err := impl.stageRolledBackUCPAPIServerCA(t.Context(), "test", RadiusSystemNamespace, history, 1)
			if tt.refusedMode == "" {
				require.True(t, called)
				require.ErrorContains(t, err, "stage attempted trust client")
				return
			}
			require.ErrorContains(t, err, "mode="+tt.refusedMode+" requires the certificate lifecycle")
			require.False(t, called)
		})
	}
}

func TestMissingSignerAPIServicePin(t *testing.T) {
	for _, name := range []string{"ucp-ca", ucpAPIServerCASecretName} {
		ca := newTestCA(t, name)
		api := ucpAPIService(base64.StdEncoding.EncodeToString(ca.pem), "True", "Passed")
		client := kubefake.NewSimpleClientset()
		impl := trustImpl(client, ucpAPIServiceDynamicClient(api))
		rendered := false
		err := impl.stageUCPAPIServerCA(t.Context(), "test", RadiusSystemNamespace, "radius", deployedMode(authz.ModeOff), authz.ModeDryRun,
			func(context.Context) (string, bool, error) { rendered = true; return "", false, nil })
		if name == "ucp-ca" {
			require.NoError(t, err)
			require.True(t, rendered)
		} else {
			require.ErrorContains(t, err, "restore the original signer")
			require.False(t, rendered)
		}
		for _, a := range client.Actions() {
			require.Equal(t, "get", a.GetVerb())
		}
	}
}
