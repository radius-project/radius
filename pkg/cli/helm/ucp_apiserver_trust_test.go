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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  []byte
}

func newTestCA(t *testing.T, name string) testCA {
	t.Helper()
	return mustTestCA(name)
}

func mustTestCA(name string) testCA {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		panic(err)
	}
	return testCA{cert: cert, key: key, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

// issueUCPCertificate returns a PEM serving certificate for UCP's Service name issued by ca.
func issueUCPCertificate(t *testing.T, ca testCA) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "ucp"},
		DNSNames:     []string{"ucp." + RadiusSystemNamespace + ".svc"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func tlsSecret(name string, crt, caCrt []byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: RadiusSystemNamespace},
		Type:       corev1.SecretTypeTLS,
		Data:       map[string][]byte{corev1.TLSCertKey: crt, corev1.TLSPrivateKeyKey: []byte("key"), caCertificateKey: caCrt},
	}
}

// apiServerCASecret is radius-apiserver-ca as cert-manager issues it from the chart's Certificate: a self-signed CA
// whose ca.crt is the CA certificate itself, annotated by secretTemplate so cainjector may copy it.
func apiServerCASecret(ca testCA) *corev1.Secret {
	secret := tlsSecret(ucpAPIServerCASecretName, ca.pem, ca.pem)
	secret.Annotations = map[string]string{allowDirectInjectionAnnotation: "true"}
	return secret
}

func ucpDeployment(rolledOut bool) *appsv1.Deployment {
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "ucp", Namespace: RadiusSystemNamespace, Generation: 2},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status:     appsv1.DeploymentStatus{ObservedGeneration: 2, Replicas: 2, UpdatedReplicas: 1, AvailableReplicas: 1},
	}
	if rolledOut {
		deployment.Status.Replicas = 1
	}
	return deployment
}

func trustImpl(kubeClient kubernetes.Interface, dynamicClient dynamic.Interface) *Impl {
	return &Impl{
		newUCPAPIServerTrustClient:  func(string) (kubernetes.Interface, error) { return kubeClient, nil },
		newCertManagerDynamicClient: func(string) (dynamic.Interface, error) { return dynamicClient, nil },
	}
}

// uninstallTrustClient returns a client for a cluster without the UCP API server trust Secret, which uninstall
// deletes.
func uninstallTrustClient(string) (kubernetes.Interface, error) {
	return kubefake.NewSimpleClientset(), nil
}

// issuedUCPAPIServerCA is a radius-apiserver-ca that cert-manager has issued.
var issuedUCPAPIServerCA = sync.OnceValue(func() []byte { return mustTestCA("radius-apiserver-ca").pem })

// issuedUCPAPIServerTrustClient returns a client for a fresh install where cert-manager has issued
// radius-apiserver-ca.
func issuedUCPAPIServerTrustClient(string) (kubernetes.Interface, error) {
	secret := tlsSecret(ucpAPIServerCASecretName, issuedUCPAPIServerCA(), issuedUCPAPIServerCA())
	secret.Annotations = map[string]string{allowDirectInjectionAnnotation: "true"}
	return kubefake.NewSimpleClientset(secret), nil
}

func shortenUCPAPIServiceWait(t *testing.T) {
	timeout, interval := ucpAPIServiceTimeout, ucpAPIServicePollInterval
	ucpAPIServiceTimeout, ucpAPIServicePollInterval = 50*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { ucpAPIServiceTimeout, ucpAPIServicePollInterval = timeout, interval })
}

// trustedSubjects returns the CommonNames of the CA certificates in the trust Secret, and checks that the Secret
// holds only ca.crt with CA certificates and allows cert-manager to inject it.
func trustedSubjects(t *testing.T, client kubernetes.Interface) []string {
	t.Helper()
	secret, err := client.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), ucpAPIServerTrustSecretName, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, "true", secret.Annotations[allowDirectInjectionAnnotation])
	require.Equal(t, []string{caCertificateKey}, keys(secret.Data))
	return pemSubjects(t, secret.Data[caCertificateKey])
}

func pemSubjects(t *testing.T, bundle []byte) []string {
	t.Helper()
	subjects := []string{}
	for block, rest := pem.Decode(bundle); block != nil; block, rest = pem.Decode(rest) {
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)
		require.True(t, cert.IsCA, "%s is not a CA certificate", cert.Subject.CommonName)
		subjects = append(subjects, cert.Subject.CommonName)
	}
	return subjects
}

func keys(data map[string][]byte) []string {
	result := []string{}
	for key := range data {
		result = append(result, key)
	}
	return result
}

func apiServiceCABundle(t *testing.T, client dynamic.Interface) []string {
	t.Helper()
	apiService, err := client.Resource(apiServicesResource).Get(t.Context(), ucpAPIServiceName, metav1.GetOptions{})
	require.NoError(t, err)
	encoded, _, _ := unstructured.NestedString(apiService.Object, "spec", "caBundle")
	bundle, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	return pemSubjects(t, bundle)
}

// apiServiceCASource returns the Secret cainjector copies into the UCP APIService.
func apiServiceCASource(t *testing.T, client dynamic.Interface) string {
	t.Helper()
	apiService, err := client.Resource(apiServicesResource).Get(t.Context(), ucpAPIServiceName, metav1.GetOptions{})
	require.NoError(t, err)
	return apiService.GetAnnotations()[injectCAFromSecretAnnotation]
}

func requireNoTrustSecret(t *testing.T, client kubernetes.Interface) {
	t.Helper()
	_, err := client.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), ucpAPIServerTrustSecretName, metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err), "Secret %s must not exist outside the upgrade overlap, got %v", ucpAPIServerTrustSecretName, err)
}

func Test_reconcileUCPAPIServerTrust_FreshInstall(t *testing.T) {
	shortenUCPAPIServiceWait(t)
	apiServerCA, orgRoot := newTestCA(t, "radius-apiserver-ca"), newTestCA(t, "org-root")
	// ucp-apiserver-tls carries an organization root in ca.crt; the API server must not trust it.
	kubeClient := kubefake.NewSimpleClientset(
		apiServerCASecret(apiServerCA),
		tlsSecret(ucpAPIServerTLSSecretName, issueUCPCertificate(t, apiServerCA), orgRoot.pem),
	)
	dynamicClient := ucpAPIServiceDynamicClient(ucpAPIService("", "", ""))

	require.NoError(t, trustImpl(kubeClient, dynamicClient).reconcileUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
	require.Equal(t, "radius-system/radius-apiserver-ca", apiServiceCASource(t, dynamicClient))
	require.Equal(t, []string{"radius-apiserver-ca"}, apiServiceCABundle(t, dynamicClient))
	requireNoTrustSecret(t, kubeClient)
}

// A rollback can restore an APIService rendered during the upgrade overlap, after rad deleted the trust Secret.
func Test_reconcileUCPAPIServerTrust_RestoredOverlapAPIService(t *testing.T) {
	shortenUCPAPIServiceWait(t)
	ucpCA, apiServerCA := newTestCA(t, "ucp-ca"), newTestCA(t, "radius-apiserver-ca")
	stale := ucpAPIService(base64.StdEncoding.EncodeToString(append(append([]byte{}, ucpCA.pem...), apiServerCA.pem...)), "True", "Passed")
	stale.SetAnnotations(map[string]string{injectCAFromSecretAnnotation: "radius-system/" + ucpAPIServerTrustSecretName})
	kubeClient := kubefake.NewSimpleClientset(apiServerCASecret(apiServerCA))
	dynamicClient := ucpAPIServiceDynamicClient(stale)

	require.NoError(t, trustImpl(kubeClient, dynamicClient).reconcileUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
	require.Equal(t, "radius-system/radius-apiserver-ca", apiServiceCASource(t, dynamicClient))
	require.Equal(t, []string{"radius-apiserver-ca"}, apiServiceCABundle(t, dynamicClient))
	requireNoTrustSecret(t, kubeClient)
}

// cainjector applies the caBundle it merged from a cached APIService, so a reconcile that read the APIService during
// the upgrade overlap can write ucp-ca back after rad switched it to radius-apiserver-ca.
func Test_reconcileUCPAPIServerTrust_CorrectsStaleCAInjectorWrite(t *testing.T) {
	shortenUCPAPIServiceWait(t)
	ucpCA, apiServerCA := newTestCA(t, "ucp-ca"), newTestCA(t, "radius-apiserver-ca")
	kubeClient := kubefake.NewSimpleClientset(apiServerCASecret(apiServerCA))
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		apiServicesResource: "APIServiceList",
	}, ucpAPIService("", "", ""))
	staleWrites := 0
	dynamicClient.PrependReactor("get", "apiservices", func(clienttesting.Action) (bool, runtime.Object, error) {
		current, err := dynamicClient.Tracker().Get(apiServicesResource, "", ucpAPIServiceName)
		require.NoError(t, err)
		apiService := current.(*unstructured.Unstructured)
		if staleWrites > 0 || apiService.GetAnnotations()[injectCAFromSecretAnnotation] != ucpAPIServerCASource(RadiusSystemNamespace) {
			return false, nil, nil
		}
		staleWrites++
		stale := apiService.DeepCopy()
		require.NoError(t, unstructured.SetNestedField(stale.Object, base64.StdEncoding.EncodeToString(append(append([]byte{}, ucpCA.pem...), apiServerCA.pem...)), "spec", "caBundle"))
		require.NoError(t, dynamicClient.Tracker().Update(apiServicesResource, stale, ""))
		return false, nil, nil
	})

	require.NoError(t, trustImpl(kubeClient, dynamicClient).reconcileUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
	require.Equal(t, 1, staleWrites)
	require.Equal(t, []string{"radius-apiserver-ca"}, apiServiceCABundle(t, dynamicClient))
}

func Test_extraCertificates(t *testing.T) {
	ca, other := newTestCA(t, "radius-apiserver-ca"), newTestCA(t, "other-ca")
	expired := newTestCA(t, "expired-ca")
	template := *expired.cert
	template.NotBefore, template.NotAfter = time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour)
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &expired.key.PublicKey, expired.key)
	require.NoError(t, err)
	expiredPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	require.Equal(t, 0, extraCertificates(ca.pem, ca.pem))
	require.Equal(t, 0, extraCertificates(append(append([]byte{}, ca.pem...), expiredPEM...), ca.pem))
	require.Equal(t, 1, extraCertificates(append(append([]byte{}, ca.pem...), other.pem...), ca.pem))
}

func Test_reconcileUCPAPIServerTrust_RejectsUnusableCASecret(t *testing.T) {
	ca, other := newTestCA(t, "radius-apiserver-ca"), newTestCA(t, "other-ca")
	leaf := issueUCPCertificate(t, ca)
	tests := []struct {
		name   string
		secret *corev1.Secret
		want   string
	}{
		{name: "tls.crt is a leaf", secret: tlsSecret(ucpAPIServerCASecretName, leaf, ca.pem), want: "its tls.crt is not a CA certificate"},
		{name: "ca.crt holds a leaf", secret: tlsSecret(ucpAPIServerCASecretName, ca.pem, append(append([]byte{}, ca.pem...), leaf...)), want: "its ca.crt does not hold only CA certificates"},
		{name: "ca.crt is another CA", secret: tlsSecret(ucpAPIServerCASecretName, ca.pem, other.pem), want: "its ca.crt does not hold the CA certificate in tls.crt"},
		{name: "cainjector may not copy ca.crt", secret: tlsSecret(ucpAPIServerCASecretName, ca.pem, ca.pem), want: "cert-manager.io/allow-direct-injection"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shortenUCPAPIServiceWait(t)
			if tt.want != "cert-manager.io/allow-direct-injection" {
				tt.secret.Annotations = map[string]string{allowDirectInjectionAnnotation: "true"}
			}
			kubeClient := kubefake.NewSimpleClientset(tt.secret)
			dynamicClient := ucpAPIServiceDynamicClient(ucpAPIService("", "", ""))

			err := trustImpl(kubeClient, dynamicClient).reconcileUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace)
			require.ErrorContains(t, err, tt.want)
			require.Empty(t, apiServiceCASource(t, dynamicClient), "rad must not point the APIService at an unusable CA")
		})
	}
}

func Test_UCPAPIServerTrust_UpgradeFromUCPCA(t *testing.T) {
	shortenUCPAPIServiceWait(t)
	ucpCA, apiServerCA := newTestCA(t, "ucp-ca"), newTestCA(t, "radius-apiserver-ca")

	// Before the upgrade the chart set the APIService caBundle to the Helm-generated ucp-ca, and UCP serves a
	// certificate from it. The bundle also holds a leaf, which must not be trusted.
	liveBundle := append(append([]byte{}, ucpCA.pem...), issueUCPCertificate(t, ucpCA)...)
	dynamicClient := ucpAPIServiceDynamicClient(ucpAPIService(base64.StdEncoding.EncodeToString(liveBundle), "True", "Passed"))
	kubeClient := kubefake.NewSimpleClientset()
	impl := trustImpl(kubeClient, dynamicClient)

	// Off does not touch the cluster.
	require.NoError(t, impl.seedUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, authz.ModeOff))
	_, err := kubeClient.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), ucpAPIServerTrustSecretName, metav1.GetOptions{})
	require.Error(t, err)

	// rad seeds the trust Secret from the live caBundle before Helm renders the APIService from it.
	require.NoError(t, impl.seedUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, authz.ModeEnforce))
	require.Equal(t, []string{"ucp-ca"}, trustedSubjects(t, kubeClient))

	// Helm installs the new CA and starts rolling UCP; the rollout has not finished, so both CAs stay trusted.
	_, err = kubeClient.CoreV1().Secrets(RadiusSystemNamespace).Create(t.Context(), apiServerCASecret(apiServerCA), metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = kubeClient.CoreV1().Secrets(RadiusSystemNamespace).Create(t.Context(), tlsSecret(ucpAPIServerTLSSecretName, issueUCPCertificate(t, apiServerCA), apiServerCA.pem), metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = kubeClient.AppsV1().Deployments(RadiusSystemNamespace).Create(t.Context(), ucpDeployment(false), metav1.CreateOptions{})
	require.NoError(t, err)

	err = impl.reconcileUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace)
	require.ErrorContains(t, err, "Deployment ucp has not finished rolling out")
	require.ErrorContains(t, err, "The API server still trusts the earlier CA too")
	require.ErrorContains(t, err, `kubectl --context "test-context" -n radius-system rollout status deployment/ucp`)
	require.Equal(t, []string{"ucp-ca", "radius-apiserver-ca"}, trustedSubjects(t, kubeClient))
	require.Equal(t, []string{"ucp-ca", "radius-apiserver-ca"}, apiServiceCABundle(t, dynamicClient))
	require.Empty(t, apiServiceCASource(t, dynamicClient), "the chart, not rad, points the APIService at the trust Secret during the overlap")

	// Once every UCP pod serves the new certificate, rad removes ucp-ca explicitly: it points the APIService at
	// radius-apiserver-ca, which cainjector follows from then on, and deletes the trust Secret.
	_, err = kubeClient.AppsV1().Deployments(RadiusSystemNamespace).UpdateStatus(t.Context(), ucpDeployment(true), metav1.UpdateOptions{})
	require.NoError(t, err)
	require.NoError(t, impl.reconcileUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
	require.Equal(t, "radius-system/radius-apiserver-ca", apiServiceCASource(t, dynamicClient))
	require.Equal(t, []string{"radius-apiserver-ca"}, apiServiceCABundle(t, dynamicClient))
	requireNoTrustSecret(t, kubeClient)

	// The next upgrade does not seed the trust Secret again.
	require.NoError(t, impl.seedUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, authz.ModeEnforce))
	requireNoTrustSecret(t, kubeClient)
}

func Test_reconcileUCPAPIServerTrust_ServingCertificateFromOtherCA(t *testing.T) {
	shortenUCPAPIServiceWait(t)
	ucpCA, apiServerCA := newTestCA(t, "ucp-ca"), newTestCA(t, "radius-apiserver-ca")
	kubeClient := kubefake.NewSimpleClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: ucpAPIServerTrustSecretName, Namespace: RadiusSystemNamespace},
			Data:       map[string][]byte{caCertificateKey: ucpCA.pem},
		},
		apiServerCASecret(apiServerCA),
		tlsSecret(ucpAPIServerTLSSecretName, issueUCPCertificate(t, ucpCA), ucpCA.pem),
		ucpDeployment(true),
	)
	dynamicClient := ucpAPIServiceDynamicClient(ucpAPIService("", "", ""))

	err := trustImpl(kubeClient, dynamicClient).reconcileUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace)
	require.ErrorContains(t, err, "is not issued by radius-apiserver-ca")
	require.Equal(t, []string{"ucp-ca", "radius-apiserver-ca"}, trustedSubjects(t, kubeClient))
	require.Empty(t, apiServiceCASource(t, dynamicClient))
}

func Test_deleteUCPAPIServerTrust(t *testing.T) {
	kubeClient := kubefake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: ucpAPIServerTrustSecretName, Namespace: RadiusSystemNamespace},
	})
	impl := trustImpl(kubeClient, ucpAPIServiceDynamicClient())

	require.NoError(t, impl.deleteUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
	_, err := kubeClient.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), ucpAPIServerTrustSecretName, metav1.GetOptions{})
	require.Error(t, err)
	// A missing Secret is not an error.
	require.NoError(t, impl.deleteUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
}
