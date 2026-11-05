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

package mtls

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/stretchr/testify/require"
)

func TestTrustBundleDigest(t *testing.T) {
	t.Parallel()
	require.Equal(t, "6134ae3055977f0d025f1c2ada38ebdcad54514560ee2819b32a4a33e03f3a9f",
		digestFingerprints([]string{strings.Repeat("0", 64), strings.Repeat("0", 63) + "1"}))
	first, second := newCA(t), newCA(t)
	want := []string{fingerprint(first.cert.Raw), fingerprint(second.cert.Raw)}
	slices.Sort(want)
	now := time.Now()
	for _, bundle := range [][]byte{
		append(bytes.Clone(first.pem), second.pem...),
		append(bytes.Clone(second.pem), first.pem...),
		append(append(bytes.Clone(second.pem), first.pem...), second.pem...),
	} {
		digest, hashes, err := TrustBundleDigest(bundle, now)
		require.NoError(t, err)
		require.Equal(t, want, hashes)
		require.Equal(t, fingerprint([]byte(strings.Join(want, "\n"))), digest)
	}
	for _, data := range [][]byte{
		nil, []byte("not PEM"), append(bytes.Clone(first.pem), []byte("PRIVATE KEY")...),
		first.issue(t, certOptions{dnsNames: componentSANs("ucp")}).certPEM,
	} {
		_, _, err := TrustBundleDigest(data, now)
		require.Error(t, err)
	}
	_, _, err := TrustBundleDigest(first.pem, first.cert.NotAfter.Add(time.Nanosecond))
	require.Error(t, err)
	var tooMany []byte
	for range maxProofCAs + 1 {
		tooMany = append(tooMany, newCA(t).pem...)
	}
	_, _, err = TrustBundleDigest(tooMany, now)
	require.ErrorContains(t, err, "too many")
}

func TestStartupProofUsesActualSourceAndRenewsLeaf(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	first := ca.issue(t, certOptions{dnsNames: componentSANs("ucp")})
	dir := newKubeletDir(t, first, ca.pem)
	source, err := NewSource(dir.root)
	require.NoError(t, err)
	digest, _, err := TrustBundleDigest(ca.pem, time.Now())
	require.NoError(t, err)
	podUID := uuid.NewString()
	proof, err := NewStartupProof(source, podUID, digest)
	require.NoError(t, err)
	before, err := proof.Snapshot()
	require.NoError(t, err)
	require.NoError(t, before.Validate(podUID, digest, fingerprint(first.cert.Raw), time.Now()))
	require.False(t, before.PeerHandshakeVerified)

	second := ca.issue(t, certOptions{dnsNames: componentSANs("ucp")})
	dir.update(t, second, ca.pem)
	after, err := proof.Snapshot()
	require.NoError(t, err)
	require.NoError(t, after.Validate(podUID, digest, fingerprint(second.cert.Raw), time.Now()))
	require.Equal(t, before.BootID, after.BootID)
	require.Equal(t, before.InitializedAt, after.InitializedAt)
	require.NotEqual(t, before.LeafSHA256, after.LeafSHA256)
	client := newTestSource(t, ca, "controller")
	result := handshake(t, source.ServerConfig(testNamespace), clientConfig(t, client, ucpServerName))
	require.NoError(t, result.clientErr)
	require.NoError(t, result.serverErr)
	require.Equal(t, second.cert.Raw, result.clientState.PeerCertificates[0].Raw)

	other := newCA(t)
	dir.update(t, other.issue(t, certOptions{dnsNames: componentSANs("ucp")}), other.pem)
	_, err = source.ClientConfig(testNamespace)
	require.Error(t, err, "transport use must reject changed roots even without an evidence query")
	_, err = proof.Snapshot()
	require.Error(t, err)
	_, err = source.load()
	require.Error(t, err, "proof mode must invalidate the actual TLS source, not keep a last-good fallback")
	dir.update(t, second, ca.pem)
	recovered, err := proof.Snapshot()
	require.NoError(t, err)
	require.Equal(t, after.BootID, recovered.BootID)
	require.Equal(t, after.LeafSHA256, recovered.LeafSHA256)
}

func TestStartupProofHTTPFailureIsPublicOnly(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	source := newTestSource(t, ca, "ucp")
	digest, _, err := TrustBundleDigest(ca.pem, time.Now())
	require.NoError(t, err)
	proof, err := NewStartupProof(source, uuid.NewString(), digest)
	require.NoError(t, err)
	serve := func(method string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		proof.ServeHTTP(w, httptest.NewRequest(method, StartupProofPath, nil))
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		return w
	}
	w := serve(http.MethodGet)
	require.Equal(t, http.StatusOK, w.Code)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &fields))
	require.Len(t, fields, 12)
	require.NotContains(t, w.Body.String(), source.dir)
	require.NotContains(t, w.Body.String(), "BEGIN")
	require.Equal(t, http.StatusMethodNotAllowed, serve(http.MethodPost).Code)
	require.NoError(t, os.WriteFile(filepath.Join(source.dir, KeyFile), []byte("private-key-sentinel"), 0o600))
	w = serve(http.MethodGet)
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.JSONEq(t, `{"schemaVersion":1,"ready":false}`, w.Body.String())
	require.NotContains(t, w.Body.String(), "sentinel")
}

func TestNewStartupProofConfiguration(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	source := newTestSource(t, ca, "ucp")
	digest, _, err := TrustBundleDigest(ca.pem, time.Now())
	require.NoError(t, err)
	for _, invalid := range []struct {
		podUID string
		digest string
	}{
		{podUID: "", digest: digest},
		{podUID: uuid.Nil.String(), digest: digest},
		{podUID: uuid.NewString(), digest: strings.Repeat("A", 64)},
		{podUID: uuid.NewString(), digest: "not-a-digest"},
		{podUID: uuid.NewString(), digest: strings.Repeat("0", 64)},
	} {
		_, err := NewStartupProof(source, invalid.podUID, invalid.digest)
		require.Error(t, err)
	}
	_, err = NewStartupProof(nil, uuid.NewString(), digest)
	require.Error(t, err)
	first, err := NewStartupProof(source, uuid.NewString(), digest)
	require.NoError(t, err)
	second, err := NewStartupProof(source, first.podUID, digest)
	require.NoError(t, err)
	require.Equal(t, first.bootID, second.bootID, "constructing another proof cannot manufacture a new process identity")
}

func TestStartupProofBoundsAndValidation(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	ca := expiringCA(t, nil, now.Add(30*time.Minute))
	other := expiringCA(t, nil, now.Add(10*time.Minute))
	bundle := append(bytes.Clone(ca.pem), other.pem...)
	dir := t.TempDir()
	leaf := ca.issue(t, certOptions{dnsNames: componentSANs("ucp"), notAfter: now.Add(time.Hour)})
	writeFiles(t, dir, leaf, bundle)
	source, err := NewSource(dir)
	require.NoError(t, err)
	source.now = func() time.Time { return now }
	digest, _, err := TrustBundleDigest(bundle, now)
	require.NoError(t, err)
	proof, err := NewStartupProof(source, uuid.NewString(), digest)
	require.NoError(t, err)
	evidence, err := proof.Snapshot()
	require.NoError(t, err)
	require.Equal(t, other.cert.NotAfter, evidence.NotAfter)
	for _, change := range []func(*StartupEvidence){
		func(e *StartupEvidence) { e.Ready = false },
		func(e *StartupEvidence) { e.SchemaVersion++ },
		func(e *StartupEvidence) { e.PodUID = uuid.NewString() },
		func(e *StartupEvidence) { e.BootID = "invalid" },
		func(e *StartupEvidence) { e.PeerHandshakeVerified = true },
		func(e *StartupEvidence) { e.CASHA256 = append(e.CASHA256, e.CASHA256[0]) },
		func(e *StartupEvidence) { e.NotAfter = now.Add(-time.Second) },
		func(e *StartupEvidence) { e.TrustBundleSHA256 = strings.Repeat("0", 64) },
	} {
		changed := evidence
		changed.CASHA256 = slices.Clone(evidence.CASHA256)
		change(&changed)
		require.Error(t, changed.Validate(evidence.PodUID, digest, fingerprint(leaf.cert.Raw), now))
	}
	source.now = func() time.Time { return other.cert.NotAfter.Add(time.Nanosecond) }
	_, err = proof.Snapshot()
	require.Error(t, err, "unchanged files must not conceal CA expiry")
	_, err = source.load()
	require.Error(t, err, "an expired loaded root must invalidate the actual source too")
}

func TestSourceOwnChainExpiresAfterLoad(t *testing.T) {
	t.Parallel()
	for _, intermediate := range []bool{false, true} {
		t.Run(map[bool]string{false: "root", true: "intermediate"}[intermediate], func(t *testing.T) {
			now := time.Now().UTC().Truncate(time.Second)
			expires := now.Add(10 * time.Minute)
			root := expiringCA(t, nil, now.Add(time.Hour))
			issuer := expiringCA(t, nil, expires)
			if intermediate {
				issuer = expiringCA(t, &root, expires)
			} else {
				root = issuer
			}

			leaf := issuer.issue(t, certOptions{dnsNames: componentSANs("ucp"), notAfter: now.Add(time.Hour)})
			if intermediate {
				leaf.certPEM = append(leaf.certPEM, issuer.pem...)
			}
			dir := t.TempDir()
			writeFiles(t, dir, leaf, root.pem)
			source, err := NewSource(dir)
			require.NoError(t, err)
			source.now = func() time.Time { return expires }
			config, err := source.ClientConfig(testNamespace)
			require.NoError(t, err)
			source.now = func() time.Time { return expires.Add(time.Nanosecond) }
			_, err = source.load()
			requireCode(t, err, "CertificateExpired")
			requireCode(t, config.VerifyConnection(tls.ConnectionState{}), "CertificateExpired")
		})
	}
}

func TestStartupProofFailureInvalidatesPooledTransport(t *testing.T) {
	t.Parallel()
	for _, http2 := range []bool{false, true} {
		t.Run(map[bool]string{false: "http1", true: "http2"}[http2], func(t *testing.T) {
			t.Parallel()
			ca := newCA(t)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			server.EnableHTTP2 = http2
			server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{
				ca.issue(t, certOptions{dnsNames: componentSANs("ucp")}).tlsCertificate(t),
			}}
			server.StartTLS()
			defer server.Close()
			leaf := ca.issue(t, certOptions{dnsNames: componentSANs("controller")})
			dir := newKubeletDir(t, leaf, ca.pem)
			transport, err := newClientTransport(authz.Options{Mode: authz.ModeEnforce,
				CertificateDirectory: dir.root, Namespace: testNamespace}, ucpServerName)
			require.NoError(t, err)
			defer transport.CloseIdleConnections()
			digest, _, err := TrustBundleDigest(ca.pem, time.Now())
			require.NoError(t, err)
			proof, err := NewStartupProof(transport.source, uuid.NewString(), digest)
			require.NoError(t, err)
			client := &http.Client{Transport: transport}
			for i := range 3 {
				var reused bool
				trace := &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused }}
				request, err := http.NewRequestWithContext(httptrace.WithClientTrace(t.Context(), trace), http.MethodGet, server.URL, nil)
				require.NoError(t, err)
				response, err := client.Do(request)
				require.NoError(t, err)
				require.NoError(t, response.Body.Close())
				require.Equal(t, i == 1, reused, "a repaired failure must not resurrect the old pooled generation")
				require.Equal(t, map[bool]int{false: 1, true: 2}[http2], response.ProtoMajor)
				if i == 1 {
					other := newCA(t)
					dir.update(t, other.issue(t, certOptions{dnsNames: componentSANs("controller")}), other.pem)
					_, err = proof.Snapshot()
					require.Error(t, err)
					dir.update(t, leaf, ca.pem)
					_, err = proof.Snapshot()
					require.NoError(t, err)
				}
			}
		})
	}
}
