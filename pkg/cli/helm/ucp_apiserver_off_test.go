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
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
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
		if dryRunAction(action) {
			continue
		}
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

	t.Run("populate an uninjected APIService", func(t *testing.T) {
		c := newOffAfterOnCluster(t)
		apiService := ucpAPIService("", "True", "Passed")
		require.NoError(t, c.dynamic.Tracker().Update(apiServicesResource, apiService, ""))
		live, err := apiServiceCACertificates(apiService)
		require.NoError(t, err)
		require.Empty(t, live)
		require.NoError(t, c.impl(nil).reconcileUCPOffAPIServerTrust(t.Context(), "test", RadiusSystemNamespace, nil))
		c.requireOffTrust(t)
	})

	for _, plainHelm := range []bool{false, true} {
		t.Run(fmt.Sprintf("removes the trust authorization on left, then changes nothing (plain Helm trust Secret %t)", plainHelm), func(t *testing.T) {
			c := newOffAfterOnCluster(t)
			c.putTrustSecret(t, plainHelm)
			c.stampSecrets(t)
			deleted := c.enforceDeletePreconditions(t, nil)
			impl := c.impl(nil)

			require.NoError(t, impl.reconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, nil))
			c.requireOffTrust(t)
			require.ElementsMatch(t, ucpOffSecretNames, *deleted, "rad must delete each Secret it inspected, pinned to its uid and resourceVersion")

			c.kube.ClearActions()
			c.dynamic.ClearActions()
			require.NoError(t, impl.reconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, nil))
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
				require.NoError(t, c.dynamic.Tracker().Add(ucpOffTestCertificate(ucpAPIServerCASecretName, ucpAPIServerCASecretName)))
				secret.UID = types.UID(name + "-2")
			case ucpAPIServerTLSSecretName:
				secret.ResourceVersion = "2"
			default:
				return
			}
			require.NoError(t, c.kube.Tracker().Update(corev1.SchemeGroupVersion.WithResource("secrets"), secret, RadiusSystemNamespace))
		})
		logs := []string{}
		impl := c.impl(nil)
		impl.logger = func(format string, v ...any) { logs = append(logs, fmt.Sprintf(format, v...)) }

		require.NoError(t, impl.reconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, nil))
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
		require.NoError(t, impl.reconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, nil))
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
		impl := c.impl(nil)
		impl.logger = func(format string, v ...any) { logs = append(logs, fmt.Sprintf(format, v...)) }

		require.NoError(t, impl.reconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, nil))
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
		require.NoError(t, impl.reconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, nil))
		require.Empty(t, c.clusterWrites())
	})

	t.Run("nothing to do without the APIService or Secret ucp-cert", func(t *testing.T) {
		c := newStageCluster(t, nil, false)
		c.onUCP(t)
		c.kube.ClearActions()
		c.dynamic.ClearActions()
		impl := c.impl(nil)
		require.NoError(t, impl.reconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, nil))
		require.Empty(t, c.clusterWrites())
	})

	t.Run("waits for UCP to serve ucp-cert and resumes", func(t *testing.T) {
		expireUCPAPIServiceWait(t)
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
		impl := c.impl(nil)

		err = impl.reconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, nil)
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
		require.NoError(t, impl.reconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, nil))
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

		require.NoError(t, c.impl(nil).reconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, nil))
		require.Equal(t, 3, patches, "rad must patch again after the conflict and after cainjector's write-back")
		require.Equal(t, 1, writeBacks)
		c.requireOffTrust(t)
	})

	// The caBundle may be clean when rad first reads it while cainjector still runs from the annotation it had, so rad
	// settles again after waiting for UCP's certificate, even though the first read needed no patch.
	t.Run("settles after the serving wait even when the caBundle starts clean", func(t *testing.T) {
		c := newOffAfterOnCluster(t)
		union, err := c.dynamic.Resource(apiServicesResource).Get(t.Context(), ucpAPIServiceName, metav1.GetOptions{})
		require.NoError(t, err)
		clean := union.DeepCopy()
		removeInjectAnnotations(clean)
		require.NoError(t, unstructured.SetNestedField(clean.Object, base64.StdEncoding.EncodeToString(c.ucpCA.pem), "spec", "caBundle"))
		require.NoError(t, c.dynamic.Tracker().Update(apiServicesResource, clean, ""))
		restored := false
		// A delayed cainjector restores the annotation and both CAs while rad waits for the ucp Deployment.
		c.kube.PrependReactor("get", "deployments", func(clienttesting.Action) (bool, runtime.Object, error) {
			if !restored {
				restored = true
				require.NoError(t, c.dynamic.Tracker().Update(apiServicesResource, union.DeepCopy(), ""))
			}
			return false, nil, nil
		})

		require.NoError(t, c.impl(nil).reconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, nil))
		require.True(t, restored)
		c.requireOffTrust(t)
	})

	// A caBundle with the off CA and any other certificate, even one that is not a CA, is not clean: the final
	// caBundle holds exactly the CA of ucp-cert.
	t.Run("replaces a caBundle with an extra leaf certificate", func(t *testing.T) {
		c := newOffAfterOnCluster(t)
		apiService, err := c.dynamic.Resource(apiServicesResource).Get(t.Context(), ucpAPIServiceName, metav1.GetOptions{})
		require.NoError(t, err)
		removeInjectAnnotations(apiService)
		withLeaf := append(append([]byte{}, c.ucpCA.pem...), issueUCPCertificate(t, c.ucpCA)...)
		require.NoError(t, unstructured.SetNestedField(apiService.Object, base64.StdEncoding.EncodeToString(withLeaf), "spec", "caBundle"))
		require.NoError(t, c.dynamic.Tracker().Update(apiServicesResource, apiService, ""))

		require.NoError(t, c.impl(nil).reconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, nil))
		c.requireOffTrust(t)
		apiService, err = c.dynamic.Resource(apiServicesResource).Get(t.Context(), ucpAPIServiceName, metav1.GetOptions{})
		require.NoError(t, err)
		encoded, _, _ := unstructured.NestedString(apiService.Object, "spec", "caBundle")
		offSecret := c.secret(t, ucpOffCertificateSecretName)
		require.Equal(t, base64.StdEncoding.EncodeToString(encodeCertificates(testCACertificates(t, offSecret.Data[caCertificateKey]))), encoded,
			"the caBundle must be exactly the PEM of the ucp-cert CA")
	})

	t.Run("keeps Secrets whose Certificate exists", func(t *testing.T) {
		c := newOffAfterOnCluster(t)
		require.NoError(t, c.dynamic.Tracker().Add(ucpOffTestCertificate(ucpAPIServerCASecretName, ucpAPIServerCASecretName)))

		require.NoError(t, c.impl(nil).reconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, nil))
		apiService, err := c.dynamic.Resource(apiServicesResource).Get(t.Context(), ucpAPIServiceName, metav1.GetOptions{})
		require.NoError(t, err)
		require.NotContains(t, apiService.GetAnnotations(), injectCAFromSecretAnnotation)
		require.Equal(t, []string{"ucp-ca"}, apiServiceCABundle(t, c.dynamic))
		_, err = c.kube.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), ucpAPIServerCASecretName, metav1.GetOptions{})
		require.NoError(t, err, "cert-manager still owns Secret %s through its Certificate", ucpAPIServerCASecretName)
		_, err = c.kube.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), ucpAPIServerTLSSecretName, metav1.GetOptions{})
		require.True(t, apierrors.IsNotFound(err), "Secret %s has no Certificate, got %v", ucpAPIServerTLSSecretName, err)
	})

	// A rollback to a chart before radius-apiserver-ca with authorization on renders the Radius release's Certificate
	// ucp-cert, which writes ucp-mtls; only a ucp-cert Certificate that still writes ucp-apiserver-tls keeps it.
	for _, tt := range []struct {
		secretName string
		keep       bool
	}{
		{secretName: "ucp-mtls", keep: false},
		{secretName: ucpAPIServerTLSSecretName, keep: true},
	} {
		t.Run("Certificate ucp-cert writes "+tt.secretName, func(t *testing.T) {
			c := newOffAfterOnCluster(t)
			certificate := ucpOffTestCertificate(ucpAPIServerCertificateName, tt.secretName)
			certificate.SetLabels(map[string]string{helmManagedByLabel: helmManagedByValue})
			certificate.SetAnnotations(map[string]string{helmReleaseNameAnnotation: radiusReleaseName, helmReleaseNamespaceAnnotation: RadiusSystemNamespace})
			require.NoError(t, c.dynamic.Tracker().Add(certificate))

			require.NoError(t, c.impl(nil).reconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, nil))
			c.requireOffAPIService(t)
			if tt.keep {
				require.NotNil(t, c.secret(t, ucpAPIServerTLSSecretName), "Certificate ucp-cert still writes Secret %s", ucpAPIServerTLSSecretName)
			} else {
				require.Nil(t, c.secret(t, ucpAPIServerTLSSecretName), "no Certificate writes Secret %s", ucpAPIServerTLSSecretName)
			}
		})
	}
}

// ucpOffTestCertificate returns a cert-manager Certificate named name that writes Secret secretName.
func ucpOffTestCertificate(name, secretName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "cert-manager.io/v1",
		"kind":       "Certificate",
		"metadata":   map[string]any{"name": name, "namespace": RadiusSystemNamespace},
		"spec":       map[string]any{"secretName": secretName},
	}}
}

func Test_ucpOffTrustClean(t *testing.T) {
	ca := newTestCA(t, "ucp-ca")
	other := newTestCA(t, "radius-apiserver-ca")
	leaf := issueUCPCertificate(t, ca)
	offCAs := testCACertificates(t, ca.pem)
	join := func(parts ...[]byte) []byte { return bytes.Join(parts, nil) }
	tests := []struct {
		name   string
		bundle []byte
		clean  bool
	}{
		{name: "exactly the off CA", bundle: ca.pem, clean: true},
		{name: "off CA and a leaf", bundle: join(ca.pem, leaf)},
		{name: "off CA and another CA", bundle: join(ca.pem, other.pem)},
		{name: "off CA and a private key block", bundle: join(ca.pem, []byte("-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----\n"))},
		{name: "off CA and an unparseable certificate", bundle: join(ca.pem, []byte("-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n"))},
		{name: "empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiService := &unstructured.Unstructured{Object: map[string]any{"spec": map[string]any{"caBundle": base64.StdEncoding.EncodeToString(tt.bundle)}}}
			clean, err := ucpOffTrustClean(apiService, offCAs)
			require.NoError(t, err)
			require.Equal(t, tt.clean, clean)
		})
	}
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

// An upgrade to authorization on that stopped before Helm leaves the staged radius-apiserver-ca chain outside the
// release. Helm's later upgrade to authorization off does not remove it, so rad does, except an object marked keep.
func Test_ReconcileUCPOffAPIServerTrust_DeletesInterruptedStage(t *testing.T) {
	shortenUCPAPIServiceWait(t)
	issuer := ucpAPIServerCAStageObjects[2]
	for _, kept := range []bool{false, true} {
		t.Run(fmt.Sprintf("reserved Issuer kept %t", kept), func(t *testing.T) {
			c := newOffAfterOnCluster(t)
			for _, staged := range ucpAPIServerCAStageObjects {
				var annotations map[string]string
				if kept && staged == issuer {
					annotations = map[string]string{helmResourcePolicyAnnotation: "keep"}
				}
				require.NoError(t, c.dynamic.Tracker().Add(stagedUCPAPIServerObject(staged.kind, staged.name, radiusReleaseName, annotations)))
			}
			logs := []string{}
			impl := c.impl(nil)
			impl.logger = func(format string, v ...any) { logs = append(logs, fmt.Sprintf(format, v...)) }

			require.NoError(t, impl.reconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, nil))
			c.requireOffTrust(t)
			for _, staged := range ucpAPIServerCAStageObjects {
				want := kept && staged == issuer
				require.Equal(t, want, stageObjectExists(t, c.dynamic, staged.resource, staged.name), "%s %s", staged.kind, staged.name)
			}
			if kept {
				require.Contains(t, strings.Join(logs, "\n"), "Warning: rad did not delete Issuer radius-system/radius-apiserver-issuer, which is not an object rad staged")
			}

			c.kube.ClearActions()
			c.dynamic.ClearActions()
			require.NoError(t, impl.reconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, nil))
			require.Empty(t, c.clusterWrites())
		})
	}
}

// A live Certificate retains only its current target.
func Test_ucpOffLeftover_CertificateSecretName(t *testing.T) {
	leftover := ucpOffLeftoverSecret{secret: ucpAPIServerTLSSecretName, certificate: ucpAPIServerCertificateName}
	certificate := func(secretName string) *unstructured.Unstructured {
		object := &unstructured.Unstructured{}
		object.SetAPIVersion("cert-manager.io/v1")
		object.SetKind("Certificate")
		object.SetName(ucpAPIServerCertificateName)
		object.SetNamespace(RadiusSystemNamespace)
		require.NoError(t, unstructured.SetNestedField(object.Object, secretName, "spec", "secretName"))
		return object
	}

	tests := []struct {
		name       string
		secretName string
		want       ucpOffSecretState
	}{
		{name: "Certificate writes the Secret", secretName: ucpAPIServerTLSSecretName, want: ucpOffSecretKeep},
		{name: "Certificate writes another Secret", secretName: "ucp-mtls", want: ucpOffSecretLeftover},
		{name: "renamed Certificate still writes the Secret", secretName: ucpAPIServerTLSSecretName, want: ucpOffSecretKeep},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeClient := kubefake.NewSimpleClientset(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{
				Name: ucpAPIServerTLSSecretName, Namespace: RadiusSystemNamespace, UID: "issued", ResourceVersion: "3",
				Annotations: map[string]string{certificateNameAnnotation: ucpAPIServerCertificateName},
			}})
			live := certificate(tt.secretName)
			if strings.HasPrefix(tt.name, "renamed") {
				live.SetName("replacement")
			}
			found, err := ucpOffLeftover(t.Context(), kubeClient, fakeCertManagerDynamicClient(nil, live), RadiusSystemNamespace, leftover)
			require.NoError(t, err)
			require.Equal(t, tt.want, found.state)
		})
	}
}

func TestHistoricalManifestOrphanCleanup(t *testing.T) {
	const manifest = `apiVersion: cert-manager.io/v1
kind: Certificate
metadata: {name: ucp-cert}
spec: {secretName: ucp-mtls}
---
apiVersion: v1
kind: Secret
metadata: {name: radius-ca}
`
	declared, err := manifestObjects(manifest)
	require.NoError(t, err)
	for _, tt := range []struct {
		name, annotation, uid, rv string
		remove                    bool
	}{
		{"ucp-mtls", "ucp-cert", "legacy", "2", false},
		{"radius-ca", "radius-ca", "declared", "3", false},
		{"ucp-spiffe", "ucp-spiffe", "issued", "4", true},
		{"dynamic-rp-spiffe", "foreign", "foreign", "5", false},
		{"applications-rp-spiffe", "", "unowned", "6", false},
		{"bicep-de-spiffe", "bicep-de-spiffe", "", "7", false},
		{"controller-mtls", "controller-cert", "unknown", "", false},
		{"operator-ca", "operator-ca", "operator", "9", false},
		{"radius-imported-ca-example", "radius-ca", "imported", "10", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			kube := kubefake.NewSimpleClientset(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{
				Namespace: RadiusSystemNamespace, Name: tt.name, UID: types.UID(tt.uid), ResourceVersion: tt.rv,
				Annotations: map[string]string{certificateNameAnnotation: tt.annotation},
			}})
			err := (&Impl{}).deleteUCPOffLeftovers(t.Context(), "", kube, fakeCertManagerDynamicClient(nil), RadiusSystemNamespace, undeclaredLeftovers(ucpOffLeftoverSecrets, declared))
			require.NoError(t, err)
			_, err = kube.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), tt.name, metav1.GetOptions{})
			if tt.remove {
				require.True(t, apierrors.IsNotFound(err))
				for _, a := range kube.Actions() {
					if del, ok := a.(clienttesting.DeleteAction); ok {
						pins := del.GetDeleteOptions().Preconditions
						require.Equal(t, types.UID(tt.uid), *pins.UID)
						require.Equal(t, tt.rv, *pins.ResourceVersion)
					}
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestHistoricalOnCleanupRequiresRollbackLease(t *testing.T) {
	const manifest = `apiVersion: cert-manager.io/v1
kind: Certificate
metadata: {name: ucp-cert}
spec: {secretName: ucp-mtls}`
	target := &releasev1.Release{Version: 1, Manifest: manifest}
	impl := &Impl{newRotationStore: uninstallTestStore(t)}
	store, err := impl.newRotationStore("test", RadiusSystemNamespace)
	require.NoError(t, err)
	secrets := store.kube.CoreV1().Secrets(store.namespace)
	for _, entry := range []ucpOffLeftoverSecret{{"ucp-mtls", "ucp-cert"}, {"ucp-spiffe", "ucp-spiffe"}} {
		_, err := secrets.Create(t.Context(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: entry.secret, Namespace: store.namespace, Annotations: map[string]string{certificateNameAnnotation: entry.certificate}}}, metav1.CreateOptions{})
		require.NoError(t, err)
	}
	cleanup := func(ctx context.Context) error {
		return impl.cleanupHistoricalOnCertificates(ctx, "test", store.namespace, target)
	}
	require.ErrorContains(t, cleanup(t.Context()), "operation Lease")
	require.NoError(t, impl.withRollbackCertificateOperation(t.Context(), "test", store.namespace, target, cleanup))
	_, err = secrets.Get(t.Context(), "ucp-mtls", metav1.GetOptions{})
	require.NoError(t, err)
	_, err = secrets.Get(t.Context(), "ucp-spiffe", metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err))
}

// removeInjectAnnotations removes the cert-manager inject annotations from apiService and keeps Helm's.
func removeInjectAnnotations(apiService *unstructured.Unstructured) {
	annotations := apiService.GetAnnotations()
	for _, annotation := range ucpInjectAnnotations {
		delete(annotations, annotation)
	}
	apiService.SetAnnotations(annotations)
}
