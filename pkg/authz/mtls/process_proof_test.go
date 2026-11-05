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
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/stretchr/testify/require"
)

func TestProcessStartupProofSharesActualListenerAndClient(t *testing.T) {
	ca := newCA(t)
	first := ca.issue(t, certOptions{dnsNames: componentSANs("applications-rp")})
	dir := newKubeletDir(t, first, ca.pem)
	options := authz.Options{Mode: authz.ModeEnforce, CertificateDirectory: dir.root, Namespace: testNamespace}
	digest, _, err := TrustBundleDigest(ca.pem, time.Now())
	require.NoError(t, err)
	t.Setenv("UCP_MTLS_STARTUP_PROOF_ENABLED", "true")
	t.Setenv("UCP_MTLS_EXPECTED_TRUST_SHA256", digest)
	t.Setenv("UCP_MTLS_POD_UID", uuid.NewString())
	proof, err := ProcessStartupProof(options, true)
	require.NoError(t, err)
	again, err := ProcessStartupProof(options, true)
	require.NoError(t, err)
	require.Same(t, proof, again)
	listenerMaterial, err := listenerSource(options)
	require.NoError(t, err)
	require.Same(t, proof.source, listenerMaterial)
	outgoing, err := newClientTransport(options, ucpServerName)
	require.NoError(t, err)
	require.Same(t, proof.source, outgoing.source)
	defer outgoing.CloseIdleConnections()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	incoming := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})}
	done := make(chan error, 1)
	go func() { done <- Serve(t.Context(), incoming, listener, options, []string{"ucp"}) }()
	t.Cleanup(func() {
		require.NoError(t, incoming.Close())
		require.ErrorIs(t, <-done, http.ErrServerClosed)
	})
	ucpLeaf := ca.issue(t, certOptions{dnsNames: componentSANs("ucp")})
	inboundClient := client(t, ca, &ucpLeaf)
	defer inboundClient.CloseIdleConnections()
	remote := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, fingerprint(r.TLS.PeerCertificates[0].Raw))
	}))
	remote.TLS = newTestSource(t, ca, "ucp").ServerConfig(testNamespace)
	remote.StartTLS()
	defer remote.Close()
	outboundClient := &http.Client{Transport: outgoing}
	var initialized time.Time
	for i := range 2 {
		leaf := first
		if i == 1 {
			leaf = ca.issue(t, certOptions{dnsNames: componentSANs("applications-rp")})
			dir.update(t, leaf, ca.pem)
			inboundClient.CloseIdleConnections()
			outgoing.CloseIdleConnections()
		}
		report, err := proof.Snapshot()
		require.NoError(t, err)
		if i == 0 {
			initialized = report.InitializedAt
		}
		require.Equal(t, initialized, report.InitializedAt)
		require.Equal(t, fingerprint(leaf.cert.Raw), report.LeafSHA256)
		response, err := inboundClient.Get("https://" + listener.Addr().String())
		require.NoError(t, err)
		require.Equal(t, http.StatusNoContent, response.StatusCode)
		require.Equal(t, report.LeafSHA256, fingerprint(response.TLS.PeerCertificates[0].Raw))
		require.NoError(t, response.Body.Close())
		response, err = outboundClient.Get(remote.URL)
		require.NoError(t, err)
		served, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		require.Equal(t, report.LeafSHA256, string(served))
	}
	dir.update(t, ca.issue(t, certOptions{dnsNames: []string{"ucp.wrong-namespace.svc"}}), ca.pem)
	_, err = outboundClient.Get(remote.URL)
	require.Error(t, err, "invalid renewed identity fails before an evidence request")
	inboundClient.CloseIdleConnections()
	_, err = inboundClient.Get("https://" + listener.Addr().String())
	require.Error(t, err, "invalid renewed identity cannot be served")
	_, err = proof.Snapshot()
	require.Error(t, err)
	dir.update(t, first, ca.pem)
	recovered, err := ProcessStartupProof(options, true)
	require.NoError(t, err)
	require.Same(t, proof, recovered)
	report, err := recovered.Snapshot()
	require.NoError(t, err)
	require.Equal(t, initialized, report.InitializedAt)
	require.Equal(t, fingerprint(first.cert.Raw), report.LeafSHA256)
	response, err := outboundClient.Get(remote.URL)
	require.NoError(t, err, "recovery must restore the actual transport, not just status")
	require.NoError(t, response.Body.Close())

	other := newCA(t)
	dir.update(t, other.issue(t, certOptions{dnsNames: componentSANs("applications-rp")}), other.pem)
	_, err = outboundClient.Get(remote.URL)
	require.Error(t, err, "dispatch rejects changed roots without waiting for a status request")
	inboundClient.CloseIdleConnections()
	_, err = inboundClient.Get("https://" + listener.Addr().String())
	require.Error(t, err)
	status := httptest.NewRecorder()
	proof.ServeHTTP(status, httptest.NewRequest(http.MethodGet, StartupProofPath, nil))
	require.Equal(t, http.StatusServiceUnavailable, status.Code)
	require.JSONEq(t, `{"schemaVersion":1,"ready":false}`, status.Body.String())
}

func TestProcessStartupProofDisabledAndInvalidConfiguration(t *testing.T) {
	t.Setenv("UCP_MTLS_STARTUP_PROOF_ENABLED", "")
	t.Setenv("UCP_MTLS_EXPECTED_TRUST_SHA256", "")
	t.Setenv("UCP_MTLS_POD_UID", "")
	proof, err := ProcessStartupProof(authz.Options{}, false)
	require.NoError(t, err)
	require.Nil(t, proof, "Off requires neither certificate files nor metrics")
	ca := newCA(t)
	options := componentOptions(t, ca, "controller", authz.ModeEnforce)
	listenerMaterial, err := listenerSource(options)
	require.NoError(t, err)
	clientMaterial, err := clientSource(options)
	require.NoError(t, err)
	require.NotSame(t, listenerMaterial, clientMaterial, "normal-mode lifetimes stay unchanged")
	digest, _, err := TrustBundleDigest(ca.pem, time.Now())
	require.NoError(t, err)
	for _, invalid := range []struct {
		name       string
		enabled    string
		digest     string
		pod        string
		mode       authz.Mode
		management bool
	}{
		{name: "invalid switch", enabled: "yes", mode: authz.ModeEnforce},
		{name: "dormant digest", digest: digest, mode: authz.ModeEnforce},
		{name: "missing digest", enabled: "true", pod: uuid.NewString(), mode: authz.ModeEnforce, management: true},
		{name: "invalid pod", enabled: "true", digest: digest, pod: "pod-name", mode: authz.ModeEnforce, management: true},
		{name: "Off", enabled: "true", digest: digest, pod: uuid.NewString(), mode: authz.ModeOff, management: true},
		{name: "no management listener", enabled: "true", digest: digest, pod: uuid.NewString(), mode: authz.ModeEnforce},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			t.Setenv("UCP_MTLS_STARTUP_PROOF_ENABLED", invalid.enabled)
			t.Setenv("UCP_MTLS_EXPECTED_TRUST_SHA256", invalid.digest)
			t.Setenv("UCP_MTLS_POD_UID", invalid.pod)
			options.Mode = invalid.mode
			proof, err := ProcessStartupProof(options, invalid.management)
			require.Error(t, err)
			require.Nil(t, proof)
		})
	}
}

func TestProcessStartupProofRejectsInvalidComponentLeaves(t *testing.T) {
	ca := newCA(t)
	digest, _, err := TrustBundleDigest(ca.pem, time.Now())
	require.NoError(t, err)
	t.Setenv("UCP_MTLS_STARTUP_PROOF_ENABLED", "true")
	t.Setenv("UCP_MTLS_EXPECTED_TRUST_SHA256", digest)
	t.Setenv("UCP_MTLS_POD_UID", uuid.NewString())
	for _, invalid := range []certOptions{
		{dnsNames: []string{"not-a-radius-component.example"}},
		{dnsNames: []string{"ucp.wrong-namespace.svc"}},
		{dnsNames: componentSANs("ucp"), extKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}},
	} {
		dir := newKubeletDir(t, ca.issue(t, invalid), ca.pem)
		_, err := ProcessStartupProof(authz.Options{Mode: authz.ModeEnforce,
			CertificateDirectory: dir.root, Namespace: testNamespace}, true)
		require.Error(t, err)
	}
}
