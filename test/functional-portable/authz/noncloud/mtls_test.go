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

package authz

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	k8s "k8s.io/client-go/kubernetes"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/radius-project/radius/test"
	"github.com/radius-project/radius/test/rogue"
	"github.com/radius-project/radius/test/rogue/rogueclient"
)

const (
	applicationsRPOperationsURL = "https://applications-rp.radius-system:5443/planes/radius/local/providers/Applications.Core/operations?api-version=2023-10-01-preview"
	logTimeout                  = time.Minute
)

func testUnauthenticatedCallToApplicationsRP(t *testing.T) {
	mode := requireMTLSMode(t)
	options := test.NewTestOptions(t)
	ctx := t.Context()

	// Mount a copy of UCP's certificate so the same pod can show that an allowed caller
	// succeeds and the failure below is caused by the missing client certificate.
	ucp, err := options.K8sClient.CoreV1().Secrets(radiusNamespace).Get(ctx, "ucp-mtls", metav1.GetOptions{})
	require.NoError(t, err)
	namespace := createNamespace(t, options.K8sClient)
	secret := createTLSSecret(t, options.K8sClient, namespace, ucp.Data["tls.crt"], ucp.Data["tls.key"], ucp.Data["ca.crt"])
	client := rogue.New(t, options.K8sClient, options.K8sConfig, rogue.Options{Namespace: namespace, TLSSecretName: secret})

	resp, err := client.Do(ctx, rogue.Request{URL: applicationsRPOperationsURL, ClientCert: true, CACert: true})
	require.NoError(t, err)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "UCP's certificate was not accepted: %+v", resp)

	// -k skips server verification so that only the missing client certificate is tested.
	resp, err = client.Do(ctx, rogue.Request{URL: applicationsRPOperationsURL, Insecure: true})
	require.NoError(t, err)
	// Enforce rejects the TLS handshake, so Go's TLS error is the reason. dryRun accepts
	// the connection and logs the missing certificate.
	reason := "no client certificate"
	if mode == authz.ModeEnforce {
		require.Equalf(t, rogueclient.ExitTransport, resp.ExitCode, "expected a TLS rejection: %+v", resp)
		require.Equal(t, 0, resp.StatusCode)
		reason = "didn't provide a certificate"
	} else {
		require.Equalf(t, http.StatusOK, resp.StatusCode, "dryRun must not reject the request: %+v", resp)
	}

	requirePeerFailureLogged(t, options.K8sClient, mode, client.Pod().Status.PodIP, reason)
}

func testDisallowedPeerKey(t *testing.T) {
	mode := requireMTLSMode(t)
	options := test.NewTestOptions(t)
	ctx := t.Context()

	ca, err := options.K8sClient.CoreV1().Secrets(radiusNamespace).Get(ctx, "radius-ca", metav1.GetOptions{})
	require.NoError(t, err)
	certPEM, keyPEM := issueRSA1024(t, ca.Data["tls.crt"], ca.Data["tls.key"])

	namespace := createNamespace(t, options.K8sClient)
	secret := createTLSSecret(t, options.K8sClient, namespace, certPEM, keyPEM, ca.Data["tls.crt"])
	client := rogue.New(t, options.K8sClient, options.K8sConfig, rogue.Options{Namespace: namespace, TLSSecretName: secret})

	resp, err := client.Do(ctx, rogue.Request{URL: applicationsRPOperationsURL, ClientCert: true, CACert: true})
	require.NoError(t, err)
	if mode == authz.ModeEnforce {
		require.Equalf(t, rogueclient.ExitTransport, resp.ExitCode, "expected a TLS rejection: %+v", resp)
		require.Equal(t, 0, resp.StatusCode)
	} else {
		require.Equalf(t, http.StatusOK, resp.StatusCode, "dryRun must not reject the request: %+v", resp)
	}

	requirePeerFailureLogged(t, options.K8sClient, mode, client.Pod().Status.PodIP, "RSA key size 1024")
}

// requireMTLSMode returns the installed authorization mode, skipping the test when it is off
// because servers do not use mTLS then.
func requireMTLSMode(t *testing.T) authz.Mode {
	t.Helper()
	mode := expectedInstalledMode(t)
	if mode == authz.ModeOff {
		t.Skip("servers use mTLS only when the authorization mode is dryRun or enforce")
	}
	return mode
}

func createNamespace(t *testing.T, client k8s.Interface) string {
	t.Helper()
	ns, err := client.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "authz-boundary-"},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) //nolint:usetesting // t.Context() is canceled before cleanup runs.
		defer cancel()
		if err := client.CoreV1().Namespaces().Delete(ctx, ns.Name, metav1.DeleteOptions{}); err != nil {
			t.Logf("failed to delete namespace %s: %v", ns.Name, err)
		}
	})
	return ns.Name
}

func createTLSSecret(t *testing.T, client k8s.Interface, namespace string, cert, key, ca []byte) string {
	t.Helper()
	secret, err := client.CoreV1().Secrets(namespace).Create(t.Context(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "rogue-tls"},
		Type:       corev1.SecretTypeTLS,
		Data:       map[string][]byte{"tls.crt": cert, "tls.key": key, "ca.crt": ca},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	return secret.Name
}

// issueRSA1024 signs a client certificate with an RSA-1024 key for the allowed caller ucp,
// so the key size is the only reason for rejection.
func issueRSA1024(t *testing.T, caCertPEM, caKeyPEM []byte) ([]byte, []byte) {
	t.Helper()

	caPair, err := tls.X509KeyPair(caCertPEM, caKeyPEM)
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(caPair.Certificate[0])
	require.NoError(t, err)

	key, err := rsa.GenerateKey(rand.Reader, 1024) //nolint:gosec // a weak key is generated to test rejection.
	require.NoError(t, err)

	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "ucp"},
		DNSNames:     []string{"ucp", "ucp." + radiusNamespace, "ucp." + radiusNamespace + ".svc"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	der, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caPair.PrivateKey)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPEM, keyPEM
}

// requirePeerFailureLogged waits for applications-rp to log a PeerCertificateInvalid denial
// (enforce) or would-deny (dryRun) for a connection from podIP whose reason contains reason.
func requirePeerFailureLogged(t *testing.T, client k8s.Interface, mode authz.Mode, podIP, reason string) {
	t.Helper()
	require.NotEmpty(t, podIP)

	marker := authz.LogFieldDenied
	if mode == authz.ModeDryRun {
		marker = authz.LogFieldWouldDeny
	}

	var last string
	err := wait.PollUntilContextTimeout(t.Context(), 2*time.Second, logTimeout, true, func(ctx context.Context) (bool, error) {
		pods, err := client.CoreV1().Pods(radiusNamespace).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=applications-rp"})
		if err != nil {
			return false, err
		}
		for _, pod := range pods.Items {
			logs := podLogs(ctx, t, client, pod.Name, "applications-rp")
			last = logs
			for _, line := range strings.Split(logs, "\n") {
				if peerFailureLine(line, marker, podIP, reason) {
					return true, nil
				}
			}
		}
		return false, nil
	})
	require.NoErrorf(t, err, "applications-rp did not log %s=%s for %s with reason %q; last logs:\n%s",
		authz.LogFieldCode, authz.CodePeerCertificateInvalid, podIP, reason, tail(last, 20))
}

func peerFailureLine(line, marker, podIP, reason string) bool {
	start := strings.IndexByte(line, '{')
	if start < 0 {
		return false
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(line[start:]), &entry); err != nil {
		return false
	}
	address, _ := entry[mtls.LogFieldPeerAddress].(string)
	got, _ := entry[mtls.LogFieldReason].(string)
	return entry[marker] == true &&
		entry[authz.LogFieldCode] == authz.CodePeerCertificateInvalid &&
		strings.HasPrefix(address, podIP+":") &&
		strings.Contains(got, reason)
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
