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
	"io"
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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/radius-project/radius/test"
	"github.com/radius-project/radius/test/rogue"
	"github.com/radius-project/radius/test/rogue/rogueclient"
)

const (
	applicationsRPOperationsPath = "/planes/radius/local/providers/Applications.Core/operations"
	applicationsRPOperationsURL  = "https://applications-rp.radius-system.svc:5443" + applicationsRPOperationsPath + "?api-version=2023-10-01-preview"
	logTimeout                   = time.Minute
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

func testDisallowedCaller(t *testing.T) {
	mode := requireMTLSMode(t)
	options := test.NewTestOptions(t)
	ctx := t.Context()

	// Positive control: UCP, the only allowed caller, gets the same route.
	ucp := componentClient(t, options, "ucp-mtls")
	resp, err := ucp.Do(ctx, rogue.Request{URL: applicationsRPOperationsURL, ClientCert: true, CACert: true})
	require.NoError(t, err)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "UCP's certificate was not accepted: %+v", resp)

	// dynamic-rp has a valid certificate from the Radius CA, but applications-rp only accepts
	// calls from UCP, so the caller check is the only reason for rejection.
	caller := "dynamic-rp." + radiusNamespace + ".svc"
	client := componentClient(t, options, "dynamic-rp-mtls")
	logs := newDenialLogs(t, options.K8sClient, mode, caller)

	resp, err = client.Do(ctx, rogue.Request{URL: applicationsRPOperationsURL, ClientCert: true, CACert: true})
	require.NoError(t, err)
	require.Equalf(t, rogueclient.ExitOK, resp.ExitCode, "expected the mTLS handshake to succeed: %+v", resp)
	if mode == authz.ModeEnforce {
		require.Equalf(t, http.StatusForbidden, resp.StatusCode, "expected dynamic-rp to be rejected: %+v", resp)
		var body v1.ErrorResponse
		require.NoErrorf(t, json.Unmarshal([]byte(resp.Body), &body), "expected an ARM error body: %+v", resp)
		require.NotNilf(t, body.Error, "expected an ARM error body: %+v", resp)
		require.Equal(t, authz.CodeAuthorizationFailed, body.Error.Code)
	} else {
		require.Equalf(t, http.StatusOK, resp.StatusCode, "dryRun must not reject the request: %+v", resp)
	}

	logs.requireNew(t)
}

// componentClient starts a rogue pod with a copy of the named Radius component mTLS Secret.
func componentClient(t *testing.T, options test.TestOptions, secretName string) *rogue.Client {
	t.Helper()
	source, err := options.K8sClient.CoreV1().Secrets(radiusNamespace).Get(t.Context(), secretName, metav1.GetOptions{})
	require.NoError(t, err)
	namespace := createNamespace(t, options.K8sClient)
	secret := createTLSSecret(t, options.K8sClient, namespace, source.Data["tls.crt"], source.Data["tls.key"], source.Data["ca.crt"])
	return rogue.New(t, options.K8sClient, options.K8sConfig, rogue.Options{Namespace: namespace, TLSSecretName: secret})
}

// denialLogs finds applications-rp AuthorizationFailed denial (enforce) or would-deny (dryRun)
// lines for a caller on the operations route. The caller check log has no peer address, so a
// match is tied to this test by reading only logs since the test started and requiring more
// matching lines than were present before the request.
type denialLogs struct {
	client   k8s.Interface
	marker   string
	caller   string
	since    metav1.Time
	baseline map[string]int
}

func newDenialLogs(t *testing.T, client k8s.Interface, mode authz.Mode, caller string) *denialLogs {
	t.Helper()
	// Log timestamps have second precision, so start one second early; the baseline
	// excludes anything already logged.
	d := &denialLogs{client: client, marker: denialMarker(mode), caller: caller, since: metav1.NewTime(time.Now().Add(-time.Second))}
	var last string
	d.baseline, last = d.count(t.Context(), t)
	t.Logf("applications-rp %s lines for %s before the request: %v; last logs:\n%s", d.marker, caller, d.baseline, tail(last, 5))
	return d
}

// denialMarker returns the log field that marks a denial in mode.
func denialMarker(mode authz.Mode) string {
	if mode == authz.ModeDryRun {
		return authz.LogFieldWouldDeny
	}
	return authz.LogFieldDenied
}

func (d *denialLogs) requireNew(t *testing.T) {
	t.Helper()
	var last string
	err := wait.PollUntilContextTimeout(t.Context(), 2*time.Second, logTimeout, true, func(ctx context.Context) (bool, error) {
		var counts map[string]int
		counts, last = d.count(ctx, t)
		for pod, n := range counts {
			if n > d.baseline[pod] {
				return true, nil
			}
		}
		return false, nil
	})
	require.NoErrorf(t, err, "applications-rp did not log %s with %s=%s for caller %s on %s after the request; last logs:\n%s",
		d.marker, authz.LogFieldCode, authz.CodeAuthorizationFailed, d.caller, applicationsRPOperationsPath, tail(last, 20))
}

func (d *denialLogs) count(ctx context.Context, t *testing.T) (map[string]int, string) {
	t.Helper()
	pods, err := d.client.CoreV1().Pods(radiusNamespace).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=applications-rp"})
	require.NoError(t, err)

	counts := map[string]int{}
	var last string
	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		stream, err := d.client.CoreV1().Pods(radiusNamespace).GetLogs(pod.Name, &corev1.PodLogOptions{Container: "applications-rp", SinceTime: &d.since}).Stream(ctx)
		require.NoError(t, err, "failed to read logs for %s", pod.Name)
		data, err := io.ReadAll(stream)
		stream.Close()
		require.NoError(t, err)

		last = string(data)
		for _, line := range strings.Split(last, "\n") {
			if d.matches(line) {
				counts[pod.Name]++
			}
		}
	}
	return counts, last
}

func (d *denialLogs) matches(line string) bool {
	entry, ok := jsonLogEntry(line)
	return ok &&
		entry[d.marker] == true &&
		entry[authz.LogFieldCode] == authz.CodeAuthorizationFailed &&
		entry[mtls.LogFieldCaller] == d.caller &&
		entry[authz.LogFieldAction] == http.MethodGet &&
		entry[authz.LogFieldTarget] == applicationsRPOperationsPath
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
		DNSNames:     []string{"ucp." + radiusNamespace + ".svc"},
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

// jsonLogEntry parses the JSON object in a log line, ignoring any prefix before it.
func jsonLogEntry(line string) (map[string]any, bool) {
	start := strings.IndexByte(line, '{')
	if start < 0 {
		return nil, false
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(line[start:]), &entry); err != nil {
		return nil, false
	}
	return entry, true
}

func peerFailureLine(line, marker, podIP, reason string) bool {
	entry, ok := jsonLogEntry(line)
	if !ok {
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

func Test_DenialLogsMatches(t *testing.T) {
	t.Parallel()

	caller := "dynamic-rp.radius-system.svc"
	line := func(marker, code, gotCaller, action, target string) string {
		data, err := json.Marshal(map[string]any{
			marker: true, authz.LogFieldCode: code, mtls.LogFieldCaller: gotCaller,
			authz.LogFieldAction: action, authz.LogFieldTarget: target,
		})
		require.NoError(t, err)
		return "2026-11-06T00:00:00Z " + string(data)
	}
	ok := line(authz.LogFieldWouldDeny, authz.CodeAuthorizationFailed, caller, http.MethodGet, applicationsRPOperationsPath)

	tests := []struct {
		name string
		mode authz.Mode
		line string
		want bool
	}{
		{name: "dryRun match", mode: authz.ModeDryRun, line: ok, want: true},
		{name: "enforce match", mode: authz.ModeEnforce, line: line(authz.LogFieldDenied, authz.CodeAuthorizationFailed, caller, http.MethodGet, applicationsRPOperationsPath), want: true},
		{name: "wrong marker for enforce", mode: authz.ModeEnforce, line: ok},
		{name: "other code", mode: authz.ModeDryRun, line: line(authz.LogFieldWouldDeny, authz.CodePeerCertificateInvalid, caller, http.MethodGet, applicationsRPOperationsPath)},
		{name: "other caller", mode: authz.ModeDryRun, line: line(authz.LogFieldWouldDeny, authz.CodeAuthorizationFailed, "controller.radius-system.svc", http.MethodGet, applicationsRPOperationsPath)},
		{name: "other target", mode: authz.ModeDryRun, line: line(authz.LogFieldWouldDeny, authz.CodeAuthorizationFailed, caller, http.MethodGet, applicationsRPOperationsPath+"/x")},
		{name: "not JSON", mode: authz.ModeDryRun, line: "authorization check would deny request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := &denialLogs{marker: denialMarker(tt.mode), caller: caller}
			require.Equal(t, tt.want, d.matches(tt.line))
		})
	}
}
