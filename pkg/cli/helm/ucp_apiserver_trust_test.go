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
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
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
	"k8s.io/apimachinery/pkg/types"
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

// helmUCPOffCertificateSecret is Secret ucp-cert as Helm applies it for the Radius release with authorization off.
func helmUCPOffCertificateSecret(crt, caCrt []byte) *corev1.Secret {
	secret := tlsSecret(ucpOffCertificateSecretName, crt, caCrt)
	setHelmOwnership(&secret.ObjectMeta, "radius", RadiusSystemNamespace)
	return secret
}

// apiServerCASecret is radius-apiserver-ca as cert-manager issues it from the chart's Certificate: a self-signed CA
// whose ca.crt is the CA certificate itself, annotated by secretTemplate so cainjector may copy it.
func apiServerCASecret(ca testCA) *corev1.Secret {
	secret := tlsSecret(ucpAPIServerCASecretName, ca.pem, ca.pem)
	secret.Annotations = issuedSecretAnnotations(ucpAPIServerCASecretName)
	secret.Annotations[allowDirectInjectionAnnotation] = "true"
	return secret
}

// issuedSecretAnnotations are the annotations of a Secret that cert-manager issued for Certificate certificate of the
// Radius release: its own certificate-name annotation and the release annotations of the chart's secretTemplate.
func issuedSecretAnnotations(certificate string) map[string]string {
	return map[string]string{certificateNameAnnotation: certificate, releaseNameAnnotation: radiusReleaseName, releaseNamespaceAnnotation: RadiusSystemNamespace}
}

// apiServerTLSSecret is ucp-apiserver-tls as cert-manager issues it for Certificate ucp-cert from radius-apiserver-ca.
func apiServerTLSSecret(t *testing.T, ca testCA) *corev1.Secret {
	t.Helper()
	secret := tlsSecret(ucpAPIServerTLSSecretName, issueUCPCertificate(t, ca), ca.pem)
	secret.Annotations = issuedSecretAnnotations(ucpAPIServerCertificateName)
	return secret
}

func ucpDeployment(rolledOut bool) *appsv1.Deployment {
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "ucp", Namespace: RadiusSystemNamespace, Generation: 2, UID: "ucp-uid",
			Labels:      map[string]string{helmManagedByLabel: helmManagedByValue},
			Annotations: map[string]string{helmReleaseNameAnnotation: radiusReleaseName, helmReleaseNamespaceAnnotation: RadiusSystemNamespace}},
		Spec:   appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{ObservedGeneration: 2, Replicas: 2, UpdatedReplicas: 1, AvailableReplicas: 1},
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

// uninstallStageClient returns a dynamic client for a cluster where rad staged nothing outside the Radius release.
func uninstallStageClient(string) (dynamic.Interface, error) {
	return fakeCertManagerDynamicClient(nil), nil
}

// issuedUCPAPIServerCA is a radius-apiserver-ca that cert-manager has issued.
var issuedUCPAPIServerCA = sync.OnceValue(func() []byte { return mustTestCA("radius-apiserver-ca").pem })

// issuedUCPAPIServerTrustClient returns a client for a fresh install where cert-manager has issued
// radius-apiserver-ca.
func issuedUCPAPIServerTrustClient(string) (kubernetes.Interface, error) {
	secret := tlsSecret(ucpAPIServerCASecretName, issuedUCPAPIServerCA(), issuedUCPAPIServerCA())
	secret.Annotations = issuedSecretAnnotations(ucpAPIServerCASecretName)
	secret.Annotations[allowDirectInjectionAnnotation] = "true"
	return kubefake.NewSimpleClientset(secret), nil
}

// shortenUCPAPIServiceWait polls every 10ms. A wait that settles needs several polls in a row, such as
// ucpAPIServerCASettleChecks clean polls after a conflict and a cainjector write-back, so the timeout leaves room for
// a slow run (for example with -race) and a test that settles still finishes in milliseconds.
func shortenUCPAPIServiceWait(t *testing.T) {
	setUCPAPIServiceWait(t, 10*time.Second)
}

// expireUCPAPIServiceWait polls every 10ms and times out after 50ms, for a test that expects a wait to time out.
func expireUCPAPIServiceWait(t *testing.T) {
	setUCPAPIServiceWait(t, 50*time.Millisecond)
}

func setUCPAPIServiceWait(t *testing.T, timeout time.Duration) {
	previousTimeout, previousInterval := ucpAPIServiceTimeout, ucpAPIServicePollInterval
	ucpAPIServiceTimeout, ucpAPIServicePollInterval = timeout, 10*time.Millisecond
	t.Cleanup(func() { ucpAPIServiceTimeout, ucpAPIServicePollInterval = previousTimeout, previousInterval })
}

// trustedSubjects returns the CommonNames of the CA certificates in the trust Secret, and checks that the Secret
// holds only ca.crt with CA certificates and allows cert-manager to inject it.
func trustedSubjects(t *testing.T, client kubernetes.Interface) []string {
	t.Helper()
	secret, err := client.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), ucpAPIServerTrustSecretName, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, trustSecretAnnotations(), secret.Annotations)
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

	require.NoError(t, trustImpl(kubeClient, dynamicClient).reconcileUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, false))
	require.Equal(t, "radius-system/radius-apiserver-ca", apiServiceCASource(t, dynamicClient))
	require.Equal(t, []string{"radius-apiserver-ca"}, apiServiceCABundle(t, dynamicClient))
	requireNoTrustSecret(t, kubeClient)
}

// A uninstall that kept radius-apiserver-trust leaves the CAs of the earlier install in it. A fresh install must neither
// trust them nor wait for UCP to move off them.
func Test_reconcileUCPAPIServerTrust_FreshInstallIgnoresLeftoverTrust(t *testing.T) {
	shortenUCPAPIServiceWait(t)
	apiServerCA, earlier := newTestCA(t, "radius-apiserver-ca"), newTestCA(t, "earlier-ca")
	leftover := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: ucpAPIServerTrustSecretName, Namespace: RadiusSystemNamespace, Annotations: trustSecretAnnotations()},
		Data:       map[string][]byte{caCertificateKey: earlier.pem},
	}
	kubeClient := kubefake.NewSimpleClientset(apiServerCASecret(apiServerCA), leftover)
	dynamicClient := ucpAPIServiceDynamicClient(ucpAPIService("", "", ""))

	require.NoError(t, trustImpl(kubeClient, dynamicClient).reconcileUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, true))
	require.Equal(t, "radius-system/radius-apiserver-ca", apiServiceCASource(t, dynamicClient))
	require.Equal(t, []string{"radius-apiserver-ca"}, apiServiceCABundle(t, dynamicClient))
	requireNoTrustSecret(t, kubeClient)
}

// A rollback can restore an APIService rendered during the upgrade overlap, after rad deleted the trust Secret.
func Test_reconcileUCPAPIServerTrust_RestoredOverlapAPIService(t *testing.T) {
	shortenUCPAPIServiceWait(t)
	ucpCA, apiServerCA := newTestCA(t, "ucp-ca"), newTestCA(t, "radius-apiserver-ca")
	stale := ucpAPIService(base64.StdEncoding.EncodeToString(append(append([]byte{}, ucpCA.pem...), apiServerCA.pem...)), "True", "Passed")
	setAnnotation(stale, injectCAFromSecretAnnotation, "radius-system/"+ucpAPIServerTrustSecretName)
	kubeClient := kubefake.NewSimpleClientset(apiServerCASecret(apiServerCA))
	dynamicClient := ucpAPIServiceDynamicClient(stale)

	require.NoError(t, trustImpl(kubeClient, dynamicClient).reconcileUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, false))
	require.Equal(t, "radius-system/radius-apiserver-ca", apiServiceCASource(t, dynamicClient))
	require.Equal(t, []string{"radius-apiserver-ca"}, apiServiceCABundle(t, dynamicClient))
	requireNoTrustSecret(t, kubeClient)
}

// cainjector applies the caBundle it merged from a cached APIService, so a reconcile that read the APIService during
// the upgrade overlap can write ucp-ca back after rad switched it to radius-apiserver-ca.
func Test_reconcileUCPAPIServerTrust_CorrectsStaleCAInjectorWrite(t *testing.T) {
	ucpCA, apiServerCA, expired := newTestCA(t, "ucp-ca"), newTestCA(t, "radius-apiserver-ca"), expiredTestCA(t, "expired-ca")
	// Each stale caBundle keeps the annotation on radius-apiserver-ca; none of them trusts only its current CA.
	staleBundles := map[string][]byte{
		"earlier CA merged back": append(append([]byte{}, ucpCA.pem...), apiServerCA.pem...),
		"empty":                  {},
		"only an expired CA":     expired,
		"current CA missing":     ucpCA.pem,
	}
	for name, staleBundle := range staleBundles {
		t.Run(name, func(t *testing.T) {
			shortenUCPAPIServiceWait(t)
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
				require.NoError(t, unstructured.SetNestedField(stale.Object, base64.StdEncoding.EncodeToString(staleBundle), "spec", "caBundle"))
				require.NoError(t, dynamicClient.Tracker().Update(apiServicesResource, stale, ""))
				return false, nil, nil
			})

			require.NoError(t, trustImpl(kubeClient, dynamicClient).reconcileUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, false))
			require.Equal(t, 1, staleWrites)
			require.Equal(t, []string{"radius-apiserver-ca"}, apiServiceCABundle(t, dynamicClient))
		})
	}
}

// expiredTestCA returns the PEM of a CA certificate that expired an hour ago.
func expiredTestCA(t *testing.T, name string) []byte {
	t.Helper()
	ca := newTestCA(t, name)
	template := *ca.cert
	template.NotBefore, template.NotAfter = time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour)
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &ca.key.PublicKey, ca.key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
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
	leaf, expired := issueUCPCertificate(t, ca), expiredTestCA(t, "radius-apiserver-ca")
	tests := []struct {
		name     string
		secret   *corev1.Secret
		annotate func(map[string]string)
		want     string
	}{
		{name: "tls.crt is a leaf", secret: tlsSecret(ucpAPIServerCASecretName, leaf, ca.pem), want: "its tls.crt is not a CA certificate"},
		{name: "ca.crt holds a leaf", secret: tlsSecret(ucpAPIServerCASecretName, ca.pem, append(append([]byte{}, ca.pem...), leaf...)), want: "its ca.crt does not hold only CA certificates"},
		{name: "ca.crt holds a non-certificate block", secret: tlsSecret(ucpAPIServerCASecretName, ca.pem,
			append(append([]byte{}, ca.pem...), "-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----\n"...)), want: "its ca.crt does not hold only CA certificates"},
		{name: "ca.crt holds trailing text", secret: tlsSecret(ucpAPIServerCASecretName, ca.pem, append(append([]byte{}, ca.pem...), "not a certificate\n"...)),
			want: "its ca.crt does not hold only CA certificates"},
		{name: "tls.crt CA is expired", secret: tlsSecret(ucpAPIServerCASecretName, expired, expired), want: "its tls.crt CA is expired or not yet valid"},
		{name: "ca.crt is another CA", secret: tlsSecret(ucpAPIServerCASecretName, ca.pem, other.pem), want: "its ca.crt does not hold the CA certificate in tls.crt"},
		{name: "cainjector may not copy ca.crt", secret: tlsSecret(ucpAPIServerCASecretName, ca.pem, ca.pem), want: "cert-manager.io/allow-direct-injection"},
		{name: "not marked for a release", secret: tlsSecret(ucpAPIServerCASecretName, ca.pem, ca.pem), want: "it does not have annotations radapp.io/release-name",
			annotate: func(a map[string]string) { delete(a, releaseNameAnnotation); delete(a, releaseNamespaceAnnotation) }},
		{name: "marked for another release", secret: tlsSecret(ucpAPIServerCASecretName, ca.pem, ca.pem), want: "it does not have annotations radapp.io/release-name",
			annotate: func(a map[string]string) { a[releaseNameAnnotation] = "other" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expireUCPAPIServiceWait(t)
			tt.secret.Annotations = issuedSecretAnnotations(ucpAPIServerCASecretName)
			if tt.want != "cert-manager.io/allow-direct-injection" {
				tt.secret.Annotations[allowDirectInjectionAnnotation] = "true"
			}
			if tt.annotate != nil {
				tt.annotate(tt.secret.Annotations)
			}
			kubeClient := kubefake.NewSimpleClientset(tt.secret)
			dynamicClient := ucpAPIServiceDynamicClient(ucpAPIService("", "", ""))

			err := trustImpl(kubeClient, dynamicClient).reconcileUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, false)
			require.ErrorContains(t, err, tt.want)
			require.Empty(t, apiServiceCASource(t, dynamicClient), "rad must not point the APIService at an unusable CA")
		})
	}
}

// noStageManifest renders a chart without radius-apiserver-ca, so staging only records the live CAs.
func noStageManifest(context.Context) (string, bool, error) { return "", true, nil }

func Test_StageUCPAPIServerCA_RefusesForeignCASecret(t *testing.T) {
	ucpCA, foreignCA := newTestCA(t, "ucp-ca"), newTestCA(t, "foreign-ca")
	for name, annotate := range map[string]func(map[string]string){
		"not marked for a release":   func(a map[string]string) { delete(a, releaseNameAnnotation); delete(a, releaseNamespaceAnnotation) },
		"marked for another release": func(a map[string]string) { a[releaseNameAnnotation] = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			// The foreign CA is otherwise usable, so only its release markers keep rad from trusting it.
			foreign := apiServerCASecret(foreignCA)
			annotate(foreign.Annotations)
			kubeClient := kubefake.NewSimpleClientset(foreign)
			dynamicClient := ucpAPIServiceDynamicClient(ucpAPIService(base64.StdEncoding.EncodeToString(ucpCA.pem), "True", "Passed"))
			before, err := dynamicClient.Resource(apiServicesResource).Get(t.Context(), ucpAPIServiceName, metav1.GetOptions{})
			require.NoError(t, err)

			stageErr := trustImpl(kubeClient, dynamicClient).stageUCPAPIServerCA(t.Context(), "test-context", RadiusSystemNamespace, "radius",
				deployedMode(authz.ModeOff), authz.ModeEnforce, noStageManifest)

			after, err := dynamicClient.Resource(apiServicesResource).Get(t.Context(), ucpAPIServiceName, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, before.Object, after.Object, "rad must not make the API server trust a foreign radius-apiserver-ca")
			_, err = kubeClient.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), ucpAPIServerTrustSecretName, metav1.GetOptions{})
			require.True(t, apierrors.IsNotFound(err), "rad must not write Secret %s", ucpAPIServerTrustSecretName)
			require.ErrorContains(t, stageErr, "Secret radius-system/radius-apiserver-ca is not marked for Helm release radius-system/radius")
			require.ErrorContains(t, stageErr, `kubectl --context "test-context" -n radius-system delete secret radius-apiserver-ca`)
		})
	}
}

func Test_UCPAPIServerTrust_UpgradeFromUCPCA(t *testing.T) {
	expireUCPAPIServiceWait(t)
	ucpCA, apiServerCA := newTestCA(t, "ucp-ca"), newTestCA(t, "radius-apiserver-ca")

	// Before the upgrade the chart set the APIService caBundle to the Helm-generated ucp-ca, and UCP serves a
	// certificate from it. The bundle also holds a leaf, which must not be trusted.
	liveBundle := append(append([]byte{}, ucpCA.pem...), issueUCPCertificate(t, ucpCA)...)
	dynamicClient := ucpAPIServiceDynamicClient(ucpAPIService(base64.StdEncoding.EncodeToString(liveBundle), "True", "Passed"))
	kubeClient := kubefake.NewSimpleClientset()
	impl := trustImpl(kubeClient, dynamicClient)

	// Off does not touch the cluster.
	require.NoError(t, impl.stageUCPAPIServerCA(t.Context(), "test-context", RadiusSystemNamespace, "radius", deployedMode(authz.ModeOff), authz.ModeOff, noStageManifest))
	_, err := kubeClient.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), ucpAPIServerTrustSecretName, metav1.GetOptions{})
	require.Error(t, err)

	// rad seeds the trust Secret from the live caBundle before Helm renders the APIService from it. This chart has no
	// radius-apiserver-ca Certificate to stage; Test_UpgradeRadius_StagesUCPAPIServerCABeforeHelm covers staging.
	require.NoError(t, impl.stageUCPAPIServerCA(t.Context(), "test-context", RadiusSystemNamespace, "radius", deployedMode(authz.ModeOff), authz.ModeEnforce, noStageManifest))
	require.Equal(t, []string{"ucp-ca"}, trustedSubjects(t, kubeClient))

	// Helm installs the new CA and starts rolling UCP; the rollout has not finished, so both CAs stay trusted.
	_, err = kubeClient.CoreV1().Secrets(RadiusSystemNamespace).Create(t.Context(), apiServerCASecret(apiServerCA), metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = kubeClient.CoreV1().Secrets(RadiusSystemNamespace).Create(t.Context(), tlsSecret(ucpAPIServerTLSSecretName, issueUCPCertificate(t, apiServerCA), apiServerCA.pem), metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = kubeClient.AppsV1().Deployments(RadiusSystemNamespace).Create(t.Context(), ucpDeployment(false), metav1.CreateOptions{})
	require.NoError(t, err)

	err = impl.reconcileUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, false)
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
	require.NoError(t, impl.reconcileUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, false))
	require.Equal(t, "radius-system/radius-apiserver-ca", apiServiceCASource(t, dynamicClient))
	require.Equal(t, []string{"radius-apiserver-ca"}, apiServiceCABundle(t, dynamicClient))
	requireNoTrustSecret(t, kubeClient)

	// The next upgrade does not seed the trust Secret again.
	require.NoError(t, impl.stageUCPAPIServerCA(t.Context(), "test-context", RadiusSystemNamespace, "radius", deployedMode(authz.ModeOff), authz.ModeEnforce, noStageManifest))
	requireNoTrustSecret(t, kubeClient)
}

func Test_reconcileUCPAPIServerTrust_ServingCertificateFromOtherCA(t *testing.T) {
	expireUCPAPIServiceWait(t)
	ucpCA, apiServerCA := newTestCA(t, "ucp-ca"), newTestCA(t, "radius-apiserver-ca")
	kubeClient := kubefake.NewSimpleClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: ucpAPIServerTrustSecretName, Namespace: RadiusSystemNamespace,
				Annotations: trustSecretAnnotations()},
			Data: map[string][]byte{caCertificateKey: ucpCA.pem},
		},
		apiServerCASecret(apiServerCA),
		tlsSecret(ucpAPIServerTLSSecretName, issueUCPCertificate(t, ucpCA), ucpCA.pem),
		ucpDeployment(true),
	)
	dynamicClient := ucpAPIServiceDynamicClient(ucpAPIService("", "", ""))

	err := trustImpl(kubeClient, dynamicClient).reconcileUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, false)
	require.ErrorContains(t, err, "is not issued by radius-apiserver-ca")
	require.Equal(t, []string{"ucp-ca", "radius-apiserver-ca"}, trustedSubjects(t, kubeClient))
	require.Empty(t, apiServiceCASource(t, dynamicClient))
}

func Test_deleteUCPAPIServerTrust(t *testing.T) {
	kubeClient := kubefake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: ucpAPIServerTrustSecretName, Namespace: RadiusSystemNamespace,
			Annotations: trustSecretAnnotations()},
		Data: map[string][]byte{caCertificateKey: newTestCA(t, "ucp-ca").pem},
	})
	impl := trustImpl(kubeClient, ucpAPIServiceDynamicClient())

	require.NoError(t, impl.deleteUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
	_, err := kubeClient.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), ucpAPIServerTrustSecretName, metav1.GetOptions{})
	require.Error(t, err)
	// A missing Secret is not an error.
	require.NoError(t, impl.deleteUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
}

// A replacement UCP pod could not start without a trust Secret its template still references, so rad keeps it until
// nothing does.
func Test_deleteUCPAPIServerTrust_KeepsReferencedSecret(t *testing.T) {
	tests := []struct {
		name string
		edit func(*corev1.PodSpec)
		kept bool
	}{
		{
			name: "projected volume",
			edit: func(spec *corev1.PodSpec) {
				spec.Volumes = append(spec.Volumes, corev1.Volume{Name: "trust", VolumeSource: corev1.VolumeSource{
					Projected: &corev1.ProjectedVolumeSource{Sources: []corev1.VolumeProjection{{Secret: &corev1.SecretProjection{
						LocalObjectReference: corev1.LocalObjectReference{Name: ucpAPIServerTrustSecretName},
					}}}},
				}})
			},
			kept: true,
		},
		{
			name: "env secretKeyRef",
			edit: func(spec *corev1.PodSpec) {
				spec.Containers = append(spec.Containers, corev1.Container{Name: "sidecar", Env: []corev1.EnvVar{{Name: "TRUST", ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: ucpAPIServerTrustSecretName}, Key: caCertificateKey},
				}}}})
			},
			kept: true,
		},
		{name: "no reference", edit: func(*corev1.PodSpec) {}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployment := ucpDeployment(true)
			tt.edit(&deployment.Spec.Template.Spec)
			kubeClient := kubefake.NewSimpleClientset(deployment, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: ucpAPIServerTrustSecretName, Namespace: RadiusSystemNamespace, Annotations: trustSecretAnnotations()},
				Data:       map[string][]byte{caCertificateKey: newTestCA(t, "ucp-ca").pem},
			})

			require.NoError(t, trustImpl(kubeClient, ucpAPIServiceDynamicClient()).deleteUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
			_, err := kubeClient.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), ucpAPIServerTrustSecretName, metav1.GetOptions{})
			if tt.kept {
				require.NoError(t, err, "rad must keep a trust Secret the ucp pod template references")
			} else {
				require.True(t, apierrors.IsNotFound(err), "got %v", err)
			}
		})
	}
}

// At the end of the upgrade overlap rad still points the APIService at radius-apiserver-ca when UCP references the
// trust Secret, and deletes the Secret on a later run once nothing does.
func Test_reconcileUCPAPIServerTrust_KeepsReferencedTrustSecret(t *testing.T) {
	shortenUCPAPIServiceWait(t)
	ucpCA, apiServerCA := newTestCA(t, "ucp-ca"), newTestCA(t, "radius-apiserver-ca")
	deployment := ucpDeployment(true)
	deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, corev1.Volume{Name: "trust",
		VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: ucpAPIServerTrustSecretName}}})
	kubeClient := kubefake.NewSimpleClientset(
		apiServerCASecret(apiServerCA),
		tlsSecret(ucpAPIServerTLSSecretName, issueUCPCertificate(t, apiServerCA), apiServerCA.pem),
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: ucpAPIServerTrustSecretName, Namespace: RadiusSystemNamespace, Annotations: trustSecretAnnotations()},
			Data:       map[string][]byte{caCertificateKey: append(append([]byte{}, ucpCA.pem...), apiServerCA.pem...)},
		},
		deployment,
	)
	dynamicClient := ucpAPIServiceDynamicClient(ucpAPIService(base64.StdEncoding.EncodeToString(append(append([]byte{}, ucpCA.pem...), apiServerCA.pem...)), "True", "Passed"))
	impl := trustImpl(kubeClient, dynamicClient)

	require.NoError(t, impl.reconcileUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, false))
	require.Equal(t, "radius-system/radius-apiserver-ca", apiServiceCASource(t, dynamicClient))
	require.Equal(t, []string{"radius-apiserver-ca"}, apiServiceCABundle(t, dynamicClient))
	_, err := kubeClient.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), ucpAPIServerTrustSecretName, metav1.GetOptions{})
	require.NoError(t, err, "rad must keep a trust Secret the ucp pod template references")

	_, err = kubeClient.AppsV1().Deployments(RadiusSystemNamespace).Update(t.Context(), ucpDeployment(true), metav1.UpdateOptions{})
	require.NoError(t, err)
	require.NoError(t, impl.reconcileUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, false))
	require.Equal(t, "radius-system/radius-apiserver-ca", apiServiceCASource(t, dynamicClient))
	require.Equal(t, []string{"radius-apiserver-ca"}, apiServiceCABundle(t, dynamicClient))
	requireNoTrustSecret(t, kubeClient)
}

// trustSecretAnnotations are the annotations rad sets on the trust Secret of the Radius release in radius-system.
func trustSecretAnnotations() map[string]string {
	return map[string]string{
		allowDirectInjectionAnnotation: "true",
		releaseNameAnnotation:          radiusReleaseName,
		releaseNamespaceAnnotation:     RadiusSystemNamespace,
	}
}

// foreignTrustSecret is a Secret named radius-apiserver-trust that neither rad nor the chart README's plain-Helm
// steps wrote: it holds other data and lacks the allow-direct-injection annotation.
func foreignTrustSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: ucpAPIServerTrustSecretName, Namespace: RadiusSystemNamespace, UID: "foreign-object", ResourceVersion: "1"},
		Data:       map[string][]byte{"unrelated-data": []byte("fixture")},
	}
}

func requireForeignTrustSecretKept(t *testing.T, client kubernetes.Interface) {
	t.Helper()
	secret, err := client.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), ucpAPIServerTrustSecretName, metav1.GetOptions{})
	require.NoError(t, err, "rad must not delete a radius-apiserver-trust Secret it does not own")
	require.Equal(t, types.UID("foreign-object"), secret.UID)
	require.Equal(t, map[string][]byte{"unrelated-data": []byte("fixture")}, secret.Data)
	require.NotContains(t, secret.Annotations, allowDirectInjectionAnnotation)
}

const trustDeleteHint = `kubectl --context "test-context" -n radius-system delete secret radius-apiserver-trust`

// stampTrustSecret gives the trust Secret the uid and resourceVersion the API server would.
func stampTrustSecret(t *testing.T, client *kubefake.Clientset) {
	t.Helper()
	secret, err := client.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), ucpAPIServerTrustSecretName, metav1.GetOptions{})
	require.NoError(t, err)
	secret.UID, secret.ResourceVersion = "trust-1", "1"
	require.NoError(t, client.Tracker().Update(corev1.SchemeGroupVersion.WithResource("secrets"), secret, RadiusSystemNamespace))
}

func Test_UCPAPIServerTrust_ForeignSecret(t *testing.T) {
	t.Run("writer refuses it and keeps its data", func(t *testing.T) {
		ucpCA := newTestCA(t, "ucp-ca")
		kubeClient := kubefake.NewSimpleClientset(foreignTrustSecret())

		_, err := writeUCPAPIServerTrust(t.Context(), kubeClient, RadiusSystemNamespace, caCertificates(ucpCA.pem))
		require.ErrorContains(t, err, "radius-apiserver-trust")
		requireForeignTrustSecretKept(t, kubeClient)
	})

	t.Run("upgrade to authorization on refuses it before Helm", func(t *testing.T) {
		shortenUCPAPIServiceWait(t)
		u := newStageUpgrade(t, map[string]any{}, "upgrade")
		c := newStageCluster(t, nil, true)
		c.addOffAPIService(t, offAPIServiceVariants[0])
		_, err := c.kube.CoreV1().Secrets(RadiusSystemNamespace).Create(t.Context(), foreignTrustSecret(), metav1.CreateOptions{})
		require.NoError(t, err)

		err = u.run(t, c.impl(u.mock(t, 0, func() {})), []string{"global.rbac.enabled=true"})
		require.ErrorContains(t, err, "Secret radius-system/radius-apiserver-trust")
		require.ErrorContains(t, err, trustDeleteHint)
		requireForeignTrustSecretKept(t, c.kube)
		require.Empty(t, c.certManagerWrites(), "rad must not stage radius-apiserver-ca when it cannot stage the trust")
	})

	t.Run("reconcile ignores it and warns", func(t *testing.T) {
		shortenUCPAPIServiceWait(t)
		apiServerCA := newTestCA(t, "radius-apiserver-ca")
		kubeClient := kubefake.NewSimpleClientset(
			foreignTrustSecret(),
			apiServerCASecret(apiServerCA),
			apiServerTLSSecret(t, apiServerCA),
			ucpDeployment(true),
		)
		dynamicClient := ucpAPIServiceDynamicClient(ucpAPIService("", "", ""))
		logs := []string{}
		impl := trustImpl(kubeClient, dynamicClient)
		impl.logger = func(format string, v ...any) { logs = append(logs, fmt.Sprintf(format, v...)) }

		require.NoError(t, impl.reconcileUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, false))
		require.Equal(t, "radius-system/radius-apiserver-ca", apiServiceCASource(t, dynamicClient))
		require.Equal(t, []string{"radius-apiserver-ca"}, apiServiceCABundle(t, dynamicClient))
		requireForeignTrustSecretKept(t, kubeClient)
		require.Len(t, logs, 1)
		require.Contains(t, logs[0], trustDeleteHint)
	})

	t.Run("uninstall keeps it and warns", func(t *testing.T) {
		kubeClient := kubefake.NewSimpleClientset(foreignTrustSecret())
		logs := []string{}
		impl := trustImpl(kubeClient, ucpAPIServiceDynamicClient())
		impl.logger = func(format string, v ...any) { logs = append(logs, fmt.Sprintf(format, v...)) }

		require.NoError(t, impl.deleteUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
		requireForeignTrustSecretKept(t, kubeClient)
		require.Len(t, logs, 1)
		require.Contains(t, logs[0], trustDeleteHint)
	})
}

// A trust Secret in the shape rad writes, but marked for another release, holds a CA that this release must not trust.
func Test_UCPAPIServerTrust_SecretOfAnotherRelease(t *testing.T) {
	otherReleaseTrust := func(ca testCA) *corev1.Secret {
		annotations := trustSecretAnnotations()
		annotations[releaseNameAnnotation] = "other"
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: ucpAPIServerTrustSecretName, Namespace: RadiusSystemNamespace, UID: "other-release", ResourceVersion: "1", Annotations: annotations},
			Data:       map[string][]byte{caCertificateKey: ca.pem},
		}
	}

	t.Run("reconcile does not trust its CA", func(t *testing.T) {
		shortenUCPAPIServiceWait(t)
		apiServerCA, otherCA := newTestCA(t, "radius-apiserver-ca"), newTestCA(t, "other-ca")
		kubeClient := kubefake.NewSimpleClientset(otherReleaseTrust(otherCA), apiServerCASecret(apiServerCA), apiServerTLSSecret(t, apiServerCA), ucpDeployment(true))
		dynamicClient := ucpAPIServiceDynamicClient(ucpAPIService("", "", ""))

		require.NoError(t, trustImpl(kubeClient, dynamicClient).reconcileUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, false))
		require.Equal(t, "radius-system/radius-apiserver-ca", apiServiceCASource(t, dynamicClient))
		require.Equal(t, []string{"radius-apiserver-ca"}, apiServiceCABundle(t, dynamicClient))
		secret, err := kubeClient.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), ucpAPIServerTrustSecretName, metav1.GetOptions{})
		require.NoError(t, err, "rad must not delete a radius-apiserver-trust Secret of another release")
		require.Equal(t, otherCA.pem, secret.Data[caCertificateKey])
	})

	t.Run("writer refuses it", func(t *testing.T) {
		otherCA := newTestCA(t, "other-ca")
		kubeClient := kubefake.NewSimpleClientset(otherReleaseTrust(otherCA))

		_, err := writeUCPAPIServerTrust(t.Context(), kubeClient, RadiusSystemNamespace, caCertificates(newTestCA(t, "ucp-ca").pem))
		require.ErrorContains(t, err, "radapp.io/release-name=radius")
		secret, err := kubeClient.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), ucpAPIServerTrustSecretName, metav1.GetOptions{})
		require.NoError(t, err)
		require.Equal(t, otherCA.pem, secret.Data[caCertificateKey])
	})
}

// An annotated Secret whose only ca.crt is empty is not in the shape rad or the plain-Helm steps write, so rad
// leaves it alone, as the chart does.
func Test_UCPAPIServerTrust_EmptyCACertificate(t *testing.T) {
	emptyTrustSecret := func() *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: ucpAPIServerTrustSecretName, Namespace: RadiusSystemNamespace, UID: "empty-object", ResourceVersion: "1",
				Annotations: trustSecretAnnotations()},
			Data: map[string][]byte{caCertificateKey: {}},
		}
	}
	requireKept := func(t *testing.T, client kubernetes.Interface) {
		t.Helper()
		secret, err := client.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), ucpAPIServerTrustSecretName, metav1.GetOptions{})
		require.NoError(t, err, "rad must not delete a radius-apiserver-trust Secret with an empty ca.crt")
		require.Equal(t, types.UID("empty-object"), secret.UID)
		require.Equal(t, map[string][]byte{caCertificateKey: {}}, secret.Data)
	}

	t.Run("is not owned", func(t *testing.T) {
		require.False(t, ownsUCPAPIServerTrust(emptyTrustSecret(), RadiusSystemNamespace))
		require.False(t, ownsUCPOffSecret(emptyTrustSecret(), ucpOffLeftoverSecrets[0]))
	})

	t.Run("writer refuses it and keeps it", func(t *testing.T) {
		kubeClient := kubefake.NewSimpleClientset(emptyTrustSecret())
		_, err := writeUCPAPIServerTrust(t.Context(), kubeClient, RadiusSystemNamespace, caCertificates(newTestCA(t, "ucp-ca").pem))
		require.ErrorContains(t, err, "radius-apiserver-trust")
		requireKept(t, kubeClient)
	})

	t.Run("uninstall keeps it and warns", func(t *testing.T) {
		kubeClient := kubefake.NewSimpleClientset(emptyTrustSecret())
		logs := []string{}
		impl := trustImpl(kubeClient, ucpAPIServiceDynamicClient())
		impl.logger = func(format string, v ...any) { logs = append(logs, fmt.Sprintf(format, v...)) }

		require.NoError(t, impl.deleteUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
		requireKept(t, kubeClient)
		require.Len(t, logs, 1)
		require.Contains(t, logs[0], trustDeleteHint)
	})
}

func Test_deleteUCPAPIServerTrust_Pinned(t *testing.T) {
	for _, plainHelm := range []bool{false, true} {
		t.Run(fmt.Sprintf("deletes the Secret rad owns, pinned to what it read (plain Helm %t)", plainHelm), func(t *testing.T) {
			c := newStageCluster(t, nil, false)
			c.putTrustSecret(t, plainHelm)
			stampTrustSecret(t, c.kube)
			deleted := c.enforceDeletePreconditions(t, nil)

			require.NoError(t, c.impl(nil).deleteUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
			require.Equal(t, []string{ucpAPIServerTrustSecretName}, *deleted)
			requireNoTrustSecret(t, c.kube)
		})
	}

	for name, change := range map[string]func(*corev1.Secret){
		"changed":   func(s *corev1.Secret) { s.ResourceVersion = "2" },
		"recreated": func(s *corev1.Secret) { s.UID = "trust-2" },
	} {
		t.Run("keeps a Secret "+name+" after rad reads it", func(t *testing.T) {
			c := newStageCluster(t, nil, false)
			c.putTrustSecret(t, false)
			stampTrustSecret(t, c.kube)
			once := false
			deleted := c.enforceDeletePreconditions(t, func(string) {
				if once {
					return
				}
				once = true
				object, err := c.kube.Tracker().Get(corev1.SchemeGroupVersion.WithResource("secrets"), RadiusSystemNamespace, ucpAPIServerTrustSecretName)
				require.NoError(t, err)
				secret := object.(*corev1.Secret).DeepCopy()
				change(secret)
				require.NoError(t, c.kube.Tracker().Update(corev1.SchemeGroupVersion.WithResource("secrets"), secret, RadiusSystemNamespace))
			})
			logs := []string{}
			impl := c.impl(nil)
			impl.logger = func(format string, v ...any) { logs = append(logs, fmt.Sprintf(format, v...)) }

			require.NoError(t, impl.deleteUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace))
			require.Empty(t, *deleted)
			require.NotNil(t, c.secret(t, ucpAPIServerTrustSecretName))
			require.Len(t, logs, 1)
			require.Contains(t, logs[0], "changed after rad read it")
		})
	}

	t.Run("reconcile deletes the trust Secret pinned to what it read", func(t *testing.T) {
		shortenUCPAPIServiceWait(t)
		c := newStageCluster(t, nil, false)
		c.putTrustSecret(t, false)
		stampTrustSecret(t, c.kube)
		c.issueAPIServerCA(t)
		_, err := c.kube.CoreV1().Secrets(RadiusSystemNamespace).Create(t.Context(), apiServerTLSSecret(t, c.apiServerCA), metav1.CreateOptions{})
		require.NoError(t, err)
		_, err = c.kube.AppsV1().Deployments(RadiusSystemNamespace).Create(t.Context(), ucpDeployment(true), metav1.CreateOptions{})
		require.NoError(t, err)
		require.NoError(t, c.dynamic.Tracker().Add(ucpAPIService("", "", "")))
		deleted := c.enforceDeletePreconditions(t, nil)

		require.NoError(t, c.impl(nil).reconcileUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, false))
		require.Equal(t, []string{ucpAPIServerTrustSecretName}, *deleted)
		require.Equal(t, []string{"radius-apiserver-ca"}, apiServiceCABundle(t, c.dynamic))
	})
}

func Test_writeUCPAPIServerTrust_RetriesConflict(t *testing.T) {
	apiServerCA := newTestCA(t, "radius-apiserver-ca")
	c := newStageCluster(t, nil, false)
	c.putTrustSecret(t, false)
	conflicts := 0
	c.kube.PrependReactor("update", "secrets", func(clienttesting.Action) (bool, runtime.Object, error) {
		if conflicts > 0 {
			return false, nil, nil
		}
		conflicts++
		return true, nil, apierrors.NewConflict(corev1.Resource("secrets"), ucpAPIServerTrustSecretName, errors.New("the object has been modified"))
	})

	_, err := writeUCPAPIServerTrust(t.Context(), c.kube, RadiusSystemNamespace, caCertificates(append(append([]byte{}, c.ucpCA.pem...), apiServerCA.pem...)))
	require.NoError(t, err)
	require.Equal(t, 1, conflicts)
	require.Equal(t, []string{"ucp-ca", "radius-apiserver-ca"}, trustedSubjects(t, c.kube))
}

// postHelmTrustPaths run the step after Helm that changes the trust of the UCP APIService, with authorization on and
// off, against a cluster whose APIService prepare may change before rad reads it.
var postHelmTrustPaths = map[string]func(t *testing.T, prepare func(*unstructured.Unstructured)) (*dynamicfake.FakeDynamicClient, func() error){
	"on": func(t *testing.T, prepare func(*unstructured.Unstructured)) (*dynamicfake.FakeDynamicClient, func() error) {
		ucpCA, apiServerCA := newTestCA(t, "ucp-ca"), newTestCA(t, "radius-apiserver-ca")
		apiService := ucpAPIService(base64.StdEncoding.EncodeToString(ucpCA.pem), "True", "Passed")
		apiService.SetUID("original")
		prepare(apiService)
		kubeClient := kubefake.NewSimpleClientset(apiServerCASecret(apiServerCA))
		dynamicClient := ucpAPIServiceDynamicClient(apiService).(*dynamicfake.FakeDynamicClient)
		return dynamicClient, func() error {
			return trustImpl(kubeClient, dynamicClient).reconcileUCPAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, false)
		}
	},
	"off": func(t *testing.T, prepare func(*unstructured.Unstructured)) (*dynamicfake.FakeDynamicClient, func() error) {
		c := newOffAfterOnCluster(t)
		current, err := c.dynamic.Tracker().Get(apiServicesResource, "", ucpAPIServiceName)
		require.NoError(t, err)
		apiService := current.(*unstructured.Unstructured).DeepCopy()
		apiService.SetUID("original")
		prepare(apiService)
		require.NoError(t, c.dynamic.Tracker().Update(apiServicesResource, apiService, ""))
		return c.dynamic, func() error {
			return c.impl(nil).ReconcileUCPOffAPIServerTrust(t.Context(), "test-context", RadiusSystemNamespace, nil)
		}
	},
}

// The APIService is cluster-scoped, so after Helm another installation may own it. rad must not change its trust.
func Test_UCPAPIServerTrust_PostHelmForeignAPIService(t *testing.T) {
	foreign := map[string]func(*unstructured.Unstructured){
		"another release namespace": func(apiService *unstructured.Unstructured) {
			setAnnotation(apiService, helmReleaseNamespaceAnnotation, "other")
		},
		"Service in another namespace": func(apiService *unstructured.Unstructured) {
			require.NoError(t, unstructured.SetNestedField(apiService.Object, "other", "spec", "service", "namespace"))
		},
	}
	for path, run := range postHelmTrustPaths {
		for name, prepare := range foreign {
			t.Run(path+"/"+name, func(t *testing.T) {
				shortenUCPAPIServiceWait(t)
				dynamicClient, reconcile := run(t, prepare)
				dynamicClient.ClearActions()

				err := reconcile()
				require.ErrorContains(t, err, "so rad will not change it")
				for _, action := range dynamicClient.Actions() {
					require.NotEqual(t, "patch", action.GetVerb(), "rad must not change an APIService that the Radius release does not own")
				}
			})
		}
	}
}

// An APIService replaced after rad checked it, even by one with the same Helm metadata, may belong to another
// installation. rad must refuse it rather than adopt it, and pin each write to the APIService it read.
func Test_UCPAPIServerTrust_PostHelmReplacedAPIService(t *testing.T) {
	for path, run := range postHelmTrustPaths {
		t.Run(path, func(t *testing.T) {
			shortenUCPAPIServiceWait(t)
			dynamicClient, reconcile := run(t, func(*unstructured.Unstructured) {})
			gets, replaced := 0, false
			dynamicClient.PrependReactor("get", "apiservices", func(clienttesting.Action) (bool, runtime.Object, error) {
				if gets++; gets == 2 {
					current, err := dynamicClient.Tracker().Get(apiServicesResource, "", ucpAPIServiceName)
					require.NoError(t, err)
					replacement := current.(*unstructured.Unstructured).DeepCopy()
					replacement.SetUID("replacement")
					require.NoError(t, dynamicClient.Tracker().Update(apiServicesResource, replacement, ""))
					replaced = true
				}
				return false, nil, nil
			})
			patches := []clienttesting.PatchAction{}
			dynamicClient.PrependReactor("patch", "apiservices", func(action clienttesting.Action) (bool, runtime.Object, error) {
				patches = append(patches, action.(clienttesting.PatchAction))
				require.False(t, replaced, "rad must not change an APIService that replaced the one it checked")
				return false, nil, nil
			})

			require.ErrorContains(t, reconcile(), "was replaced")
			for _, patch := range patches {
				require.Contains(t, string(patch.GetPatch()), `"original"`, "each patch must carry the uid rad read")
				require.Contains(t, string(patch.GetPatch()), "resourceVersion", "each patch must carry the resourceVersion rad read")
			}
		})
	}
}
