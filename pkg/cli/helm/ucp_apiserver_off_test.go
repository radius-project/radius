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
	"fmt"
	"strings"
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

// ucpOffSecretNames are the Secrets of the radius-apiserver-ca chain that ReconcileUCPOffAPIServerTrust may delete.
var ucpOffSecretNames = []string{ucpAPIServerTrustSecretName, ucpAPIServerCASecretName, ucpAPIServerTLSSecretName}

// putTrustSecret writes radius-apiserver-trust as rad does, or as the chart README's plain Helm steps create it.
func (c *stageCluster) putTrustSecret(t *testing.T, plainHelm bool) {
	t.Helper()
	secrets := c.kube.CoreV1().Secrets(RadiusSystemNamespace)
	if !plainHelm {
		_, err := writeUCPAPIServerTrust(t.Context(), c.kube, RadiusSystemNamespace, testCACertificates(t, c.ucpCA.pem))
		require.NoError(t, err)
		return
	}
	err := secrets.Delete(t.Context(), ucpAPIServerTrustSecretName, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		require.NoError(t, err)
	}
	_, err = secrets.Create(t.Context(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: ucpAPIServerTrustSecretName, Namespace: RadiusSystemNamespace,
			Annotations: map[string]string{allowDirectInjectionAnnotation: "true"}},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{caCertificateKey: c.ucpCA.pem},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
}

// stampSecrets gives the Secrets the uid and resourceVersion the API server would, which the fake clientset does not.
func (c *stageCluster) stampSecrets(t *testing.T) {
	t.Helper()
	for _, name := range ucpOffSecretNames {
		secret, err := c.kube.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), name, metav1.GetOptions{})
		require.NoError(t, err)
		secret.UID = types.UID(name + "-1")
		secret.ResourceVersion = "1"
		require.NoError(t, c.kube.Tracker().Update(corev1.SchemeGroupVersion.WithResource("secrets"), secret, RadiusSystemNamespace))
	}
}

// enforceDeletePreconditions makes the fake clientset reject a Secret delete without matching uid and resourceVersion
// preconditions, as the API server does when they no longer match. beforeDelete runs first, so a test can change the
// Secret between rad's read and its delete. It returns the Secrets deleted.
func (c *stageCluster) enforceDeletePreconditions(t *testing.T, beforeDelete func(name string)) *[]string {
	t.Helper()
	deleted := []string{}
	c.kube.PrependReactor("delete", "secrets", func(action clienttesting.Action) (bool, runtime.Object, error) {
		del := action.(clienttesting.DeleteActionImpl)
		if beforeDelete != nil {
			beforeDelete(del.Name)
		}
		object, err := c.kube.Tracker().Get(corev1.SchemeGroupVersion.WithResource("secrets"), del.Namespace, del.Name)
		if err != nil {
			return false, nil, nil
		}
		secret := object.(*corev1.Secret)
		preconditions := del.DeleteOptions.Preconditions
		if preconditions == nil || preconditions.UID == nil || *preconditions.UID != secret.UID ||
			preconditions.ResourceVersion == nil || *preconditions.ResourceVersion != secret.ResourceVersion {
			return true, nil, apierrors.NewConflict(corev1.Resource("secrets"), del.Name, fmt.Errorf("precondition failed"))
		}
		deleted = append(deleted, del.Name)
		return false, nil, nil
	})
	return &deleted
}

func (c *stageCluster) secret(t *testing.T, name string) *corev1.Secret {
	t.Helper()
	secret, err := c.kube.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	require.NoError(t, err)
	return secret
}

func (c *stageCluster) requireOffAPIService(t *testing.T) {
	t.Helper()
	apiService, err := c.dynamic.Resource(apiServicesResource).Get(t.Context(), ucpAPIServiceName, metav1.GetOptions{})
	require.NoError(t, err)
	require.NotContains(t, apiService.GetAnnotations(), injectCAFromSecretAnnotation)
	require.Equal(t, []string{"ucp-ca"}, apiServiceCABundle(t, c.dynamic))
}

func Test_ReconcileUCPOffAPIServerTrust(t *testing.T) {
	shortenUCPAPIServiceWait(t)

	for _, plainHelm := range []bool{false, true} {
		t.Run(fmt.Sprintf("removes the trust authorization on left, then changes nothing (plain Helm trust Secret %t)", plainHelm), func(t *testing.T) {
			c := newOffAfterOnCluster(t)
			c.putTrustSecret(t, plainHelm)
			c.stampSecrets(t)
			deleted := c.enforceDeletePreconditions(t, nil)
			impl := c.impl(nil, "")

			require.NoError(t, impl.ReconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
			c.requireOffTrust(t)
			require.ElementsMatch(t, ucpOffSecretNames, *deleted, "rad must delete each Secret it inspected, pinned to its uid and resourceVersion")

			c.kube.ClearActions()
			c.dynamic.ClearActions()
			require.NoError(t, impl.ReconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
			require.Empty(t, c.clusterWrites())
		})
	}

	t.Run("keeps a Secret that changes or is recreated after rad reads it", func(t *testing.T) {
		c := newOffAfterOnCluster(t)
		c.putTrustSecret(t, false)
		c.stampSecrets(t)
		changed := map[string]bool{}
		c.enforceDeletePreconditions(t, func(name string) {
			if changed[name] {
				return
			}
			changed[name] = true
			// The reactor runs under the fake clientset's lock, so read through the tracker.
			object, err := c.kube.Tracker().Get(corev1.SchemeGroupVersion.WithResource("secrets"), RadiusSystemNamespace, name)
			require.NoError(t, err)
			secret := object.(*corev1.Secret).DeepCopy()
			switch name {
			case ucpAPIServerCASecretName:
				// A concurrent rad upgrade to authorization on issues a new radius-apiserver-ca from a new Certificate.
				require.NoError(t, c.dynamic.Tracker().Add(&unstructured.Unstructured{Object: map[string]any{
					"apiVersion": "cert-manager.io/v1",
					"kind":       "Certificate",
					"metadata":   map[string]any{"name": ucpAPIServerCASecretName, "namespace": RadiusSystemNamespace},
				}}))
				secret.UID = types.UID(name + "-2")
			case ucpAPIServerTLSSecretName:
				secret.ResourceVersion = "2"
			default:
				return
			}
			require.NoError(t, c.kube.Tracker().Update(corev1.SchemeGroupVersion.WithResource("secrets"), secret, RadiusSystemNamespace))
		})
		logs := []string{}
		impl := c.impl(nil, "")
		impl.logger = func(format string, v ...any) { logs = append(logs, fmt.Sprintf(format, v...)) }

		require.NoError(t, impl.ReconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
		c.requireOffAPIService(t)
		require.Nil(t, c.secret(t, ucpAPIServerTrustSecretName))
		recreated := c.secret(t, ucpAPIServerCASecretName)
		require.NotNil(t, recreated, "the recreated %s must survive", ucpAPIServerCASecretName)
		require.Equal(t, types.UID(ucpAPIServerCASecretName+"-2"), recreated.UID)
		updated := c.secret(t, ucpAPIServerTLSSecretName)
		require.NotNil(t, updated, "the changed %s must survive", ucpAPIServerTLSSecretName)
		require.Equal(t, "2", updated.ResourceVersion)
		require.Contains(t, strings.Join(logs, "\n"), "Secret radius-system/ucp-apiserver-tls changed")

		// A rerun deletes the changed Secret, which is still a leftover, and keeps the one its new Certificate owns.
		require.NoError(t, impl.ReconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
		require.Nil(t, c.secret(t, ucpAPIServerTLSSecretName))
		require.NotNil(t, c.secret(t, ucpAPIServerCASecretName))
	})

	t.Run("keeps Secrets it does not own and warns", func(t *testing.T) {
		c := newOffAfterOnCluster(t)
		secrets := c.kube.CoreV1().Secrets(RadiusSystemNamespace)
		ca := c.secret(t, ucpAPIServerCASecretName)
		ca.Annotations["cert-manager.io/certificate-name"] = "other"
		_, err := secrets.Update(t.Context(), ca, metav1.UpdateOptions{})
		require.NoError(t, err)
		tls := c.secret(t, ucpAPIServerTLSSecretName)
		tls.Annotations = nil
		_, err = secrets.Update(t.Context(), tls, metav1.UpdateOptions{})
		require.NoError(t, err)
		c.putTrustSecret(t, true)
		trust := c.secret(t, ucpAPIServerTrustSecretName)
		trust.Annotations = nil
		_, err = secrets.Update(t.Context(), trust, metav1.UpdateOptions{})
		require.NoError(t, err)
		logs := []string{}
		impl := c.impl(nil, "")
		impl.logger = func(format string, v ...any) { logs = append(logs, fmt.Sprintf(format, v...)) }

		require.NoError(t, impl.ReconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
		c.requireOffAPIService(t)
		for _, name := range ucpOffSecretNames {
			require.NotNil(t, c.secret(t, name), "rad must not delete Secret %s, which it does not own", name)
			require.Contains(t, strings.Join(logs, "\n"),
				fmt.Sprintf(`Warning: rad did not delete Secret radius-system/%s`, name))
			require.Contains(t, strings.Join(logs, "\n"),
				fmt.Sprintf(`kubectl --context "test-context" -n radius-system delete secret %s`, name))
		}

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

	patch, err := ucpOffTrustPatch(apiService, testCACertificates(t, ca.pem))
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
