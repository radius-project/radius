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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	valuesutil "helm.sh/helm/v4/pkg/chart/common/util"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	k8stesting "k8s.io/client-go/testing"
)

func TestBootstrapIssuancePreparation(t *testing.T) {
	for _, mode := range []string{"selfSigned", "caSecret", "issuerRef"} {
		for _, external := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/external=%t", mode, external), func(t *testing.T) {
				s, _ := bootstrapStoreFixture(t)
				selected, err := loader.Load("../../../deploy/Chart")
				require.NoError(t, err)
				key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
				require.NoError(t, err)
				ca, private, certificate := testSigningCA(t, key, nil, nil, nil)
				policy := newIssuerPolicyFixture(t)
				issuer := policy.object("Issuer", "external-ca")
				issuer.SetUID("issuer-uid")
				issuer.SetResourceVersion("1")
				server := policy.server(t)
				kubeContext := useTargetKubeconfig(t, server.URL, server.URL, false)
				values := valuesutil.CoalesceTables(copyValues(policy.values), copyValues(selected.Values))
				settings := map[string]any{"mode": mode}
				if mode == "caSecret" {
					settings["caSecret"], settings["caBundle"] = "operator-ca", string(ca)
					_, err = s.kube.CoreV1().Secrets(s.namespace).Create(t.Context(),
						caTLSSecret("operator-ca", ca, private), metav1.CreateOptions{})
					require.NoError(t, err)
				} else if mode == "issuerRef" {
					settings = certificateValues(values)
					settings["caBundle"] = string(ca)
					require.NoError(t, s.dynamic.(*dynamicfake.FakeDynamicClient).Tracker().Add(issuer.DeepCopy()))
				}
				setCertificateValues(values, settings)
				require.NoError(t, unstructured.SetNestedField(values, true, "de", "ucpMTLS", "enabled"))
				require.NoError(t, unstructured.SetNestedField(values, true, "global", "prometheus", "enabled"))
				require.NoError(t, unstructured.SetNestedField(values, int64(9090), "global", "prometheus", "port"))
				if external {
					require.NoError(t, unstructured.SetNestedField(values, "external", "global", "rbac", "mtls", "provider"))
					values["de"] = externalProviderValues()["de"]
				}
				b := &aggregationBootstrapInstaller{impl: &Impl{
					newCertificateClient: func(string) (kubernetes.Interface, error) { return s.kube, nil },
				}, store: s, chart: selected, values: values, options: NewDefaultClusterOptions(), kubeContext: kubeContext,
					runner: &rotationRunner{store: s}}
				s.verifyBootstrapIssuerPolicy = func(ctx context.Context, state aggregationBootstrapState) error {
					return b.impl.checkIssuerPolicy(ctx, b.options, selected, nil, values, kubeContext, false)
				}
				state, err := b.requested(t.Context())
				require.NoError(t, err)
				s.activeBootstrap = state.ID
				_, _, err = s.beginBootstrap(t.Context(), state)
				require.NoError(t, err)
				s.dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "certificates", func(action k8stesting.Action) (bool, runtime.Object, error) {
					object := action.(k8stesting.CreateAction).GetObject().(*unstructured.Unstructured)
					name, _, err := unstructured.NestedString(object.Object, "spec", "secretName")
					require.NoError(t, err)
					crt, signingKey := ca, private
					if name != "radius-ca" {
						crt, signingKey, _ = testSigningCA(t, nil, func(leaf *x509.Certificate) {
							leaf.IsCA, leaf.KeyUsage = false, x509.KeyUsageDigitalSignature
							leaf.DNSNames = []string{"ucp." + s.namespace + ".svc"}
							leaf.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
							leaf.NotAfter = time.Now().Add(24 * time.Hour)
						}, certificate, key)
					}
					_, err = s.kube.CoreV1().Secrets(s.namespace).Create(t.Context(), &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.namespace},
						Type:       corev1.SecretTypeTLS, Data: map[string][]byte{"tls.crt": crt, "tls.key": signingKey},
					}, metav1.CreateOptions{})
					require.NoError(t, err)
					return false, nil, nil
				})
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				require.NoError(t, b.prepare(ctx, state))
				_, staged, err := s.readBootstrap(t.Context())
				require.NoError(t, err)
				require.Equal(t, bootstrapInstalling, staged.Phase)
				require.NotEmpty(t, staged.TrustSHA256)
				require.NotEmpty(t, staged.InstallValuesSHA256)
				require.NoError(t, s.checkBootstrapLeaf(t.Context(), staged))
				require.NoError(t, s.checkBootstrapResources(t.Context(), staged))
				if mode == "selfSigned" {
					require.NotEmpty(t, staged.RootSecretUID)
					require.NotEqual(t, "radius-ca", staged.Signer, "the renewing root must not replace the retained selected signer")
				}
				if mode == "issuerRef" {
					require.Empty(t, staged.Signer)
					require.Len(t, staged.Resources, 1, "external issuance must not create local component PKI or issuers")
				}
				_, err = s.dynamic.Resource(apiServiceResource).Get(t.Context(), radiusAPIService, metav1.GetOptions{})
				require.True(t, apierrors.IsNotFound(err), "public trust is not exposed before the installing phase")
			})
		}
	}
}
