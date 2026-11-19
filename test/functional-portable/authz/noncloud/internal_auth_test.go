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
	"encoding/json"
	"io"
	"net"
	"net/http"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	k8s "k8s.io/client-go/kubernetes"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test"
	"github.com/radius-project/radius/test/rogue"
	"github.com/radius-project/radius/test/rogue/rogueclient"
)

const (
	applicationsRPOperationsPath = "/planes/radius/local/providers/Applications.Core/operations"
	applicationsRPOperationsURL  = "https://applications-rp.radius-system.svc:5443" + applicationsRPOperationsPath + "?api-version=2023-10-01-preview"
	dynamicRPURL                 = "https://dynamic-rp.radius-system.svc:8082/"
	logTimeout                   = time.Minute
)

func testUnauthenticatedCallToApplicationsRP(t *testing.T) {
	mode := requireInternalAuthMode(t)
	options := test.NewTestOptions(t)
	ctx := t.Context()

	client := newVerifiedClient(t, options, map[string]string{
		allowedTokenDir: serviceAccountToken(t, options.K8sClient, "ucp", "applications-rp"),
	})

	// Positive control: the same pod succeeds with UCP's token, so the failure below is
	// caused by the missing token.
	client.requireAllowed(t)

	// No token is sent, so -k is safe here; only the receiver is tested.
	resp, err := client.Do(ctx, rogue.Request{URL: applicationsRPOperationsURL, Insecure: true})
	require.NoError(t, err)
	requireRejected(t, mode, resp, http.StatusUnauthorized, v1.CodeInvalidAuthenticationInfo)
	requireAuthFailureLogged(t, options.K8sClient, mode, client.Pod().Status.PodIP, "missing bearer token")
}

func testDisallowedCaller(t *testing.T) {
	mode := requireInternalAuthMode(t)
	options := test.NewTestOptions(t)
	ctx := t.Context()

	// dynamic-rp's token has applications-rp's audience, but applications-rp only accepts
	// calls from UCP, so the caller check is the only reason for rejection.
	const wrongCaller = "wrong-caller"
	client := newVerifiedClient(t, options, map[string]string{
		allowedTokenDir: serviceAccountToken(t, options.K8sClient, "ucp", "applications-rp"),
		wrongCaller:     serviceAccountToken(t, options.K8sClient, "dynamic-rp", "applications-rp"),
	})
	client.requireAllowed(t)

	caller := "dynamic-rp." + radiusNamespace + ".svc"
	logs := newDenialLogs(t, options.K8sClient, mode, caller)

	resp, err := client.Do(ctx, client.request(applicationsRPOperationsURL, wrongCaller))
	require.NoError(t, err)
	requireRejected(t, mode, resp, http.StatusForbidden, authz.CodeAuthorizationFailed)
	logs.requireNew(t)
}

func testTokenReplay(t *testing.T) {
	mode := requireInternalAuthMode(t)
	options := test.NewTestOptions(t)
	ctx := t.Context()

	// UCP's token for dynamic-rp, replayed to applications-rp. The caller is allowed, so the
	// audience is the only reason for rejection.
	const replayed = "replayed"
	client := newVerifiedClient(t, options, map[string]string{
		allowedTokenDir: serviceAccountToken(t, options.K8sClient, "ucp", "applications-rp"),
		replayed:        serviceAccountToken(t, options.K8sClient, "ucp", "dynamic-rp"),
	})
	client.requireAllowed(t)

	resp, err := client.Do(ctx, client.request(applicationsRPOperationsURL, replayed))
	require.NoError(t, err)
	requireRejected(t, mode, resp, http.StatusUnauthorized, v1.CodeInvalidAuthenticationInfo)
	requireAuthFailureLogged(t, options.K8sClient, mode, client.Pod().Status.PodIP, "token is not valid")
}

func testWrongServerSPIFFEID(t *testing.T) {
	requireInternalAuthMode(t)
	options := test.NewTestOptions(t)
	ctx := t.Context()

	// The rogue pod uses the Radius client transport with the Radius CA bundle and UCP's
	// token for applications-rp, like UCP itself.
	client := newVerifiedClient(t, options, map[string]string{
		allowedTokenDir: serviceAccountToken(t, options.K8sClient, "ucp", "applications-rp"),
	})

	// Positive control: applications-rp presents the expected SPIFFE ID.
	client.requireAllowed(t)

	// dynamic-rp has a valid server certificate from the Radius CA, but its SPIFFE ID is not
	// applications-rp's, so the client must fail the handshake before sending the token.
	resp, err := client.Do(ctx, client.request(dynamicRPURL, allowedTokenDir))
	require.NoError(t, err)
	require.Equalf(t, rogueclient.ExitTransport, resp.ExitCode, "expected the client to refuse dynamic-rp: %+v", resp)
	require.Equal(t, 0, resp.StatusCode)
	require.Contains(t, resp.Stderr, authz.CodePeerCertificateInvalid)
	require.Contains(t, resp.Stderr, mtls.SPIFFEID(client.trustDomain, radiusNamespace, "dynamic-rp"))
}

// allowedTokenDir holds UCP's token for applications-rp in a verifiedClient pod.
const allowedTokenDir = "allowed"

// verifiedClient is a rogue pod that calls applications-rp like UCP does: it verifies the
// server against the Radius CA bundle and applications-rp's SPIFFE ID in the installed trust
// domain, and only then sends a token read from a mounted file. Tokens never appear on the
// rogue-client command line, which is sent in the pods/exec URL.
type verifiedClient struct {
	*rogue.Client
	trustDomain string
}

// newVerifiedClient starts a verifiedClient whose pod mounts each token in tokens under
// rogue.TLSMountPath/<directory>/applications-rp/token, where directory is the map key.
func newVerifiedClient(t *testing.T, options test.TestOptions, tokens map[string]string) *verifiedClient {
	t.Helper()
	ctx := t.Context()

	ca, err := options.K8sClient.CoreV1().Secrets(radiusNamespace).Get(ctx, "radius-ca", metav1.GetOptions{})
	require.NoError(t, err)

	data := map[string][]byte{"ca.crt": ca.Data["ca.crt"]}
	items := []corev1.KeyToPath{{Key: "ca.crt", Path: "ca.crt"}}
	for dir, token := range tokens {
		key := dir + "-token"
		data[key] = []byte(token)
		items = append(items, corev1.KeyToPath{Key: key, Path: path.Join(dir, "applications-rp", "token")})
	}

	namespace := createNamespace(t, options.K8sClient)
	secret, err := options.K8sClient.CoreV1().Secrets(namespace).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "rogue-identity"},
		Data:       data,
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	return &verifiedClient{
		Client: rogue.New(t, options.K8sClient, options.K8sConfig, rogue.Options{
			Namespace:      namespace,
			TLSSecretName:  secret.Name,
			TLSSecretItems: items,
		}),
		trustDomain: installedTrustDomain(installedValues(t)),
	}
}

// request returns a request to url that expects applications-rp's SPIFFE ID and sends the
// token mounted under tokenDir.
func (c *verifiedClient) request(url, tokenDir string) rogue.Request {
	return rogue.Request{
		URL:               url,
		Receiver:          "applications-rp",
		ReceiverNamespace: radiusNamespace,
		TrustDomain:       c.trustDomain,
		TokenDirectory:    tokenDir,
	}
}

// requireAllowed checks that applications-rp accepts the operations route with UCP's token
// after the client verified applications-rp's identity.
func (c *verifiedClient) requireAllowed(t *testing.T) {
	t.Helper()
	resp, err := c.Do(t.Context(), c.request(applicationsRPOperationsURL, allowedTokenDir))
	require.NoError(t, err)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "UCP's token was not accepted: %+v", resp)
}

// serviceAccountToken requests a 10-minute token for a ServiceAccount in the Radius namespace
// with the given audience, as the kubelet does for the projected token volumes.
func serviceAccountToken(t *testing.T, client k8s.Interface, account, audience string) string {
	t.Helper()
	request, err := client.CoreV1().ServiceAccounts(radiusNamespace).CreateToken(t.Context(), account, &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			Audiences:         []string{audience},
			ExpirationSeconds: to.Ptr(int64(600)),
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	return request.Status.Token
}

// requireRejected checks that enforce rejected the request with status and an ARM error
// code, and that dryRun served it.
func requireRejected(t *testing.T, mode authz.Mode, resp rogue.Response, status int, code string) {
	t.Helper()
	require.Equalf(t, rogueclient.ExitOK, resp.ExitCode, "expected an HTTP response: %+v", resp)
	if mode != authz.ModeEnforce {
		require.Equalf(t, http.StatusOK, resp.StatusCode, "dryRun must not reject the request: %+v", resp)
		return
	}
	require.Equalf(t, status, resp.StatusCode, "expected the request to be rejected: %+v", resp)
	var body v1.ErrorResponse
	require.NoErrorf(t, json.Unmarshal([]byte(resp.Body), &body), "expected an ARM error body: %+v", resp)
	require.NotNilf(t, body.Error, "expected an ARM error body: %+v", resp)
	require.Equal(t, code, body.Error.Code)
}

// denialLogs finds applications-rp AuthorizationFailed denial (enforce) or would-deny (dryRun)
// lines for a caller on the operations route. The caller check log has no peer address, so a
// match is tied to this test by requiring more matching lines in a pod than that pod had before
// the request. Logs are not filtered by time: SinceTime is compared with the node clock, and host
// clock skew could exclude the denial line.
type denialLogs struct {
	client   k8s.Interface
	marker   string
	caller   string
	baseline map[string]int
}

func newDenialLogs(t *testing.T, client k8s.Interface, mode authz.Mode, caller string) *denialLogs {
	t.Helper()
	d := &denialLogs{client: client, marker: denialMarker(mode), caller: caller}
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
		stream, err := d.client.CoreV1().Pods(radiusNamespace).GetLogs(pod.Name, &corev1.PodLogOptions{Container: "applications-rp"}).Stream(ctx)
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

// requireInternalAuthMode returns the installed authorization mode, skipping the test when it
// is off because servers do not authenticate callers then.
func requireInternalAuthMode(t *testing.T) authz.Mode {
	t.Helper()
	mode := expectedInstalledMode(t)
	if mode == authz.ModeOff {
		t.Skip("servers authenticate callers only when the authorization mode is dryRun or enforce")
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

// requireAuthFailureLogged waits for applications-rp to log an InvalidAuthenticationInfo denial
// (enforce) or would-deny (dryRun) for a request from podIP whose reason contains reason.
func requireAuthFailureLogged(t *testing.T, client k8s.Interface, mode authz.Mode, podIP, reason string) {
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
				if authFailureLine(line, marker, podIP, reason) {
					return true, nil
				}
			}
		}
		return false, nil
	})
	require.NoErrorf(t, err, "applications-rp did not log %s=%s for %s with reason %q; last logs:\n%s",
		authz.LogFieldCode, v1.CodeInvalidAuthenticationInfo, podIP, reason, tail(last, 20))
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

func authFailureLine(line, marker, podIP, reason string) bool {
	entry, ok := jsonLogEntry(line)
	if !ok {
		return false
	}
	address, _ := entry[mtls.LogFieldPeerAddress].(string)
	got, _ := entry[mtls.LogFieldReason].(string)
	return entry[marker] == true &&
		entry[authz.LogFieldCode] == v1.CodeInvalidAuthenticationInfo &&
		peerIsPod(address, podIP) &&
		strings.Contains(got, reason)
}

// peerIsPod reports whether address, a host:port peer address that brackets IPv6 hosts,
// comes from podIP.
func peerIsPod(address, podIP string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip, want := net.ParseIP(host), net.ParseIP(podIP)
	return ip != nil && want != nil && ip.Equal(want)
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func Test_AuthFailureLine(t *testing.T) {
	t.Parallel()

	marker := authz.LogFieldDenied
	line := func(address string) string {
		data, err := json.Marshal(map[string]any{
			marker: true, authz.LogFieldCode: v1.CodeInvalidAuthenticationInfo,
			mtls.LogFieldPeerAddress: address, mtls.LogFieldReason: "missing bearer token",
		})
		require.NoError(t, err)
		return "2026-11-06T00:00:00Z " + string(data)
	}

	tests := []struct {
		name    string
		podIP   string
		address string
		want    bool
	}{
		{name: "IPv4", podIP: "10.0.0.1", address: "10.0.0.1:43210", want: true},
		{name: "IPv6", podIP: "fd00::1", address: "[fd00::1]:43210", want: true},
		{name: "IPv6 non-canonical pod IP", podIP: "fd00:0:0:0:0:0:0:1", address: "[fd00::1]:43210", want: true},
		{name: "IPv4 other pod with shared prefix", podIP: "10.0.0.1", address: "10.0.0.12:43210"},
		{name: "IPv6 other pod", podIP: "fd00::1", address: "[fd00::2]:43210"},
		{name: "no port", podIP: "10.0.0.1", address: "10.0.0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, authFailureLine(line(tt.address), marker, tt.podIP, "missing bearer token"))
		})
	}
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
