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
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clienttesting "k8s.io/client-go/testing"
)

// newOffAfterOnCluster returns a cluster that Helm just switched from authorization on to off: UCP serves the
// certificate in Secret ucp-cert, but the APIService keeps rad's inject-ca-from-secret annotation, cainjector merged
// radius-apiserver-ca into the caBundle, and the Secrets of the radius-apiserver-ca chain remain.
func newOffAfterOnCluster(t *testing.T) *stageCluster {
	t.Helper()
	c := newStageCluster(t, nil, false)
	c.onUCP(t)
	c.helmAppliesOff(t, nil, nil)
	require.Equal(t, []string{"ucp-ca", "radius-apiserver-ca"}, apiServiceCABundle(t, c.dynamic))
	return c
}

// clusterWrites returns the write requests sent to either client of c.
func (c *stageCluster) clusterWrites() []string {
	writes := []string{}
	for _, action := range append(c.kube.Actions(), c.dynamic.Actions()...) {
		switch action.GetVerb() {
		case "get", "list", "watch":
		default:
			writes = append(writes, action.GetVerb()+" "+action.GetResource().Resource)
		}
	}
	return writes
}

func Test_ReconcileUCPOffAPIServerTrust(t *testing.T) {
	shortenUCPAPIServiceWait(t)

	t.Run("removes the trust authorization on left, then changes nothing", func(t *testing.T) {
		c := newOffAfterOnCluster(t)
		impl := c.impl(nil, "")

		require.NoError(t, impl.ReconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
		c.requireOffTrust(t)

		c.kube.ClearActions()
		c.dynamic.ClearActions()
		require.NoError(t, impl.ReconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
		require.Empty(t, c.clusterWrites())
	})

	t.Run("nothing to do without the APIService or Secret ucp-cert", func(t *testing.T) {
		c := newStageCluster(t, nil, false)
		c.onUCP(t)
		c.kube.ClearActions()
		c.dynamic.ClearActions()
		impl := c.impl(nil, "")
		require.NoError(t, impl.ReconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
		require.Empty(t, c.clusterWrites())
	})

	t.Run("waits for UCP to serve ucp-cert and resumes", func(t *testing.T) {
		c := newStageCluster(t, nil, false)
		c.onUCP(t)
		// Helm wrote Secret ucp-cert and the off caBundle, but UCP still serves the certificate from radius-apiserver-ca.
		_, err := c.kube.CoreV1().Secrets(RadiusSystemNamespace).Create(t.Context(),
			tlsSecret(ucpOffCertificateSecretName, issueUCPCertificate(t, c.ucpCA), c.ucpCA.pem), metav1.CreateOptions{})
		require.NoError(t, err)
		apiService, err := c.dynamic.Resource(apiServicesResource).Get(t.Context(), ucpAPIServiceName, metav1.GetOptions{})
		require.NoError(t, err)
		require.NoError(t, unstructured.SetNestedField(apiService.Object,
			base64.StdEncoding.EncodeToString(c.cainjectorMerges(t, apiService, c.ucpCA.pem)), "spec", "caBundle"))
		_, err = c.dynamic.Resource(apiServicesResource).Update(t.Context(), apiService, metav1.UpdateOptions{})
		require.NoError(t, err)
		impl := c.impl(nil, "")

		err = impl.ReconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace)
		require.ErrorContains(t, err, "UCP does not serve the certificate in Secret ucp-cert")
		require.ErrorContains(t, err, `kubectl --context "test-context" -n radius-system rollout status deployment/ucp`)
		// The API server still trusts the certificate UCP serves, and nothing that issues it is removed.
		c.requireTrusts(t, c.servedLeaf(t), "UCP")
		require.Equal(t, []string{"ucp-ca", "radius-apiserver-ca"}, apiServiceCABundle(t, c.dynamic))
		apiService, err = c.dynamic.Resource(apiServicesResource).Get(t.Context(), ucpAPIServiceName, metav1.GetOptions{})
		require.NoError(t, err)
		require.Contains(t, apiService.GetAnnotations(), injectCAFromSecretAnnotation)
		for _, name := range []string{ucpAPIServerCASecretName, ucpAPIServerTLSSecretName} {
			_, err := c.kube.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), name, metav1.GetOptions{})
			require.NoError(t, err, "Secret %s must remain while UCP serves from it", name)
		}

		deployment, err := c.kube.AppsV1().Deployments(RadiusSystemNamespace).Get(t.Context(), "ucp", metav1.GetOptions{})
		require.NoError(t, err)
		deployment.Spec.Template.Spec.Volumes = []corev1.Volume{{Name: ucpCertificateVolumeName,
			VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: ucpOffCertificateSecretName}}}}
		_, err = c.kube.AppsV1().Deployments(RadiusSystemNamespace).Update(t.Context(), deployment, metav1.UpdateOptions{})
		require.NoError(t, err)
		require.NoError(t, impl.ReconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
		c.requireOffTrust(t)
	})

	t.Run("retries when cainjector writes first", func(t *testing.T) {
		c := newOffAfterOnCluster(t)
		union, err := c.dynamic.Resource(apiServicesResource).Get(t.Context(), ucpAPIServiceName, metav1.GetOptions{})
		require.NoError(t, err)
		patches, writeBacks := 0, 0
		// The first patch loses to a cainjector write, as the resourceVersion test does on a real API server.
		c.dynamic.PrependReactor("patch", "apiservices", func(clienttesting.Action) (bool, runtime.Object, error) {
			patches++
			if patches == 1 {
				return true, nil, apierrors.NewConflict(schema.GroupResource{Group: "apiregistration.k8s.io", Resource: "apiservices"}, ucpAPIServiceName, nil)
			}
			return false, nil, nil
		})
		// After the patch that succeeds, cainjector writes back the caBundle and annotation it read earlier once.
		c.dynamic.PrependReactor("get", "apiservices", func(clienttesting.Action) (bool, runtime.Object, error) {
			if patches == 2 && writeBacks == 0 {
				writeBacks++
				require.NoError(t, c.dynamic.Tracker().Update(apiServicesResource, union.DeepCopy(), ""))
			}
			return false, nil, nil
		})

		require.NoError(t, c.impl(nil, "").ReconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
		require.Equal(t, 3, patches, "rad must patch again after the conflict and after cainjector's write-back")
		require.Equal(t, 1, writeBacks)
		c.requireOffTrust(t)
	})

	t.Run("keeps Secrets whose Certificate exists", func(t *testing.T) {
		c := newOffAfterOnCluster(t)
		certificate := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Certificate",
			"metadata":   map[string]any{"name": ucpAPIServerCASecretName, "namespace": RadiusSystemNamespace},
		}}
		require.NoError(t, c.dynamic.Tracker().Add(certificate))

		require.NoError(t, c.impl(nil, "").ReconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
		apiService, err := c.dynamic.Resource(apiServicesResource).Get(t.Context(), ucpAPIServiceName, metav1.GetOptions{})
		require.NoError(t, err)
		require.NotContains(t, apiService.GetAnnotations(), injectCAFromSecretAnnotation)
		require.Equal(t, []string{"ucp-ca"}, apiServiceCABundle(t, c.dynamic))
		_, err = c.kube.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), ucpAPIServerCASecretName, metav1.GetOptions{})
		require.NoError(t, err, "cert-manager still owns Secret %s through its Certificate", ucpAPIServerCASecretName)
		_, err = c.kube.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), ucpAPIServerTLSSecretName, metav1.GetOptions{})
		require.True(t, apierrors.IsNotFound(err), "Secret %s has no Certificate, got %v", ucpAPIServerTLSSecretName, err)
	})
}

func Test_ucpOffTrustPatch(t *testing.T) {
	ca := newTestCA(t, "ucp-ca")
	apiService := ucpAPIService("", "", "")
	apiService.SetUID(types.UID("uid-1"))
	apiService.SetResourceVersion("42")
	apiService.SetAnnotations(map[string]string{
		injectCAFromSecretAnnotation:     "radius-system/radius-apiserver-ca",
		"cert-manager.io/inject-ca-from": "radius-system/ucp-cert",
		"example.com/kept":               "value",
	})

	patch, err := ucpOffTrustPatch(apiService, caCertificates(ca.pem))
	require.NoError(t, err)
	var operations []map[string]any
	require.NoError(t, json.Unmarshal(patch, &operations))
	require.Equal(t, []map[string]any{
		{"op": "test", "path": "/metadata/uid", "value": "uid-1"},
		{"op": "test", "path": "/metadata/resourceVersion", "value": "42"},
		{"op": "remove", "path": "/metadata/annotations/cert-manager.io~1inject-ca-from"},
		{"op": "remove", "path": "/metadata/annotations/cert-manager.io~1inject-ca-from-secret"},
		{"op": "add", "path": "/spec/caBundle", "value": base64.StdEncoding.EncodeToString(ca.pem)},
	}, operations)
}
