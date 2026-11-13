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

package helm

import (
	"context"
	"crypto/x509"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestInitialCertificateNativeControllerFixtures(t *testing.T) {
	store, config, _, _ := newRotationTestStore(t)
	require.NoError(t, store.dynamic.Resource(apiServiceResource).Delete(t.Context(), radiusAPIService, metav1.DeleteOptions{}))
	installRotationTestCRDs(t, store)
	crds := store.dynamic.Resource(schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"})
	crd, err := crds.Get(t.Context(), "certificates.cert-manager.io", metav1.GetOptions{})
	require.NoError(t, err)
	versions, _, err := unstructured.NestedSlice(crd.Object, "spec", "versions")
	require.NoError(t, err)
	version := versions[0].(map[string]any)
	require.NoError(t, unstructured.SetNestedMap(version, map[string]any{"type": "object", "x-kubernetes-preserve-unknown-fields": true},
		"schema", "openAPIV3Schema", "properties", "status"))
	require.NoError(t, unstructured.SetNestedMap(version, map[string]any{}, "subresources", "status"))
	require.NoError(t, unstructured.SetNestedSlice(crd.Object, versions, "spec", "versions"))
	_, err = crds.Update(t.Context(), crd, metav1.UpdateOptions{})
	require.NoError(t, err)
	kubeconfig := filepath.Join(t.TempDir(), "kubeconfig")
	require.NoError(t, clientcmd.WriteToFile(clientcmdapi.Config{
		Clusters:  map[string]*clientcmdapi.Cluster{"fixture": {Server: config.Host, CertificateAuthorityData: config.CAData}},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{"fixture": {ClientCertificateData: config.CertData, ClientKeyData: config.KeyData}},
		Contexts:  map[string]*clientcmdapi.Context{"fixture": {Cluster: "fixture", AuthInfo: "fixture"}}, CurrentContext: "fixture",
	}, kubeconfig))
	t.Setenv("KUBECONFIG", kubeconfig)
	selected, err := loader.Load("../../../deploy/Chart")
	require.NoError(t, err)
	for _, provider := range []string{"radius", "external"} {
		t.Run(provider, func(t *testing.T) {
			local := *store
			local.namespace = "initial-" + provider
			options := NewDefaultClusterOptions()
			options.Radius.Namespace, options.Radius.Timeout = local.namespace, time.Minute
			values := map[string]any{"global": map[string]any{
				"rbac":       map[string]any{"enabled": true, "tls": map[string]any{"provider": provider}},
				"prometheus": map[string]any{"enabled": true, "port": 9090},
			}}
			impl := &Impl{newRotationStore: func(string, string) (*rotationStore, error) { return &local, nil }}
			target, plan, err := impl.prepareCertificateTarget(t.Context(), options, selected, values, "fixture")
			require.NoError(t, err)
			b, err := impl.prepareAggregationBootstrap(t.Context(), options, selected, nil, values, "fixture", plan, target)
			require.NoError(t, err)
			fixtures := map[string]*corev1.Secret{}
			for _, plane := range b.planeNames() {
				name := "radius-apiserver-ca"
				if plane == "internal" {
					name = "radius-ca"
				}
				ca, key, _ := testSigningCA(t, nil, func(c *x509.Certificate) { c.Subject.CommonName = name }, nil, nil)
				secret := caTLSSecret(name, ca, key)
				secret.Namespace, secret.Data[caCertificateKey] = local.namespace, ca
				secret.Annotations = map[string]string{"cert-manager.io/certificate-name": name,
					"cert-manager.io/issuer-name": "radius-selfsigned-bootstrap", "cert-manager.io/issuer-kind": "Issuer",
					"cert-manager.io/issuer-group": "cert-manager.io"}
				fixtures[name] = secret
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			issued := make(chan error, 1)
			// This is an explicit controller fixture using the isolated API server,
			// not evidence of cert-manager issuance, CSI, or workload acceptance.
			go func() {
				issued <- wait.PollUntilContextCancel(ctx, 10*time.Millisecond, true, func(ctx context.Context) (bool, error) {
					for name, secret := range fixtures {
						client := local.dynamic.Resource(rotationCertificateResource).Namespace(local.namespace)
						certificate, err := client.Get(ctx, name, metav1.GetOptions{})
						if apierrors.IsNotFound(err) {
							continue
						}
						if err != nil {
							return false, err
						}
						_, err = local.kube.CoreV1().Secrets(local.namespace).Create(ctx, secret, metav1.CreateOptions{})
						if err != nil {
							return false, err
						}
						certificate.Object["status"] = map[string]any{"conditions": []any{map[string]any{
							"type": "Ready", "status": "True", "observedGeneration": certificate.GetGeneration(),
						}}}
						if _, err := client.UpdateStatus(ctx, certificate, metav1.UpdateOptions{}); err != nil {
							return false, err
						}
						delete(fixtures, name)
					}
					return len(fixtures) == 0, nil
				})
			}()
			b.applyFixture = func(ctx context.Context) error {
				manifest, err := impl.renderBootstrapChart(ctx, selected, options, b.values, "fixture")
				if err != nil {
					return err
				}
				bundle, err := renderedAggregationTrust(manifest, local.namespace)
				require.Equal(t, b.state.Anchors["aggregation"].CA, bundle)
				require.NoError(t, err)
				changed := copyValues(b.values)
				changed["fixture-change"] = true
				_, err = impl.renderBootstrapChart(ctx, selected, options, changed, "fixture")
				require.ErrorContains(t, err, "recorded values")
				b.state.Phase = "preparing"
				require.NoError(t, b.save(ctx))
				_, err = impl.renderBootstrapChart(ctx, selected, options, b.values, "fixture")
				require.ErrorContains(t, err, "Finish initial certificate public-source publication")
				b.state.Phase = "published"
				err = b.save(ctx)
				return err
			}
			b.verifyFixture = func(ctx context.Context) error { return b.checkMaterial(ctx) }
			runErr := b.run(ctx)
			cancel()
			controllerErr := <-issued
			require.NoError(t, controllerErr)
			require.NoError(t, runErr)
			require.Equal(t, "complete", b.state.Phase)
			source, err := local.kube.CoreV1().Secrets(local.namespace).Get(t.Context(), ucpAPIServerTrustSecretName, metav1.GetOptions{})
			require.NoError(t, err)
			require.Len(t, source.Data, 1)
		})
	}
}
