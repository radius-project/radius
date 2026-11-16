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
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const ucpServerName = "ucp." + testNamespace + ".svc"

func componentSANs(component string) []string {
	return []string{component + "." + testNamespace + ".svc"}
}

func TestAggregationProofUsesServingSource(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	first := ca.issue(t, certOptions{dnsNames: componentSANs("ucp"), extKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
	dir := newKubeletDir(t, first, ca.pem)
	source, err := NewSource(dir.root)
	require.NoError(t, err)
	digest, _, err := TrustBundleDigest(ca.pem, time.Now())
	require.NoError(t, err)
	uid := uuid.NewString()
	proof, err := NewAggregationProof(source, testNamespace, uid, digest)
	require.NoError(t, err, "server-only aggregation needs no component client certificate")
	before, err := proof.Snapshot()
	require.NoError(t, err)
	require.NoError(t, before.Validate(uid, digest, fingerprint(first.cert.Raw), time.Now()))
	require.Error(t, StartupEvidence(before).Validate(uid, digest, fingerprint(first.cert.Raw), time.Now()))
	require.Error(t, source.bindProofNamespace(testNamespace), "aggregation Source cannot become component proof")
	roots := x509.NewCertPool()
	roots.AddCert(ca.cert)
	client := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: ucpServerName}
	result := handshake(t, source.ServingConfig(nil), client)
	require.NoError(t, result.clientErr)
	require.NoError(t, result.serverErr)
	require.Equal(t, first.cert.Raw, result.clientState.PeerCertificates[0].Raw)

	next := ca.issue(t, certOptions{dnsNames: componentSANs("ucp"), extKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
	dir.update(t, next, ca.pem)
	after, err := proof.Snapshot()
	require.NoError(t, err)
	require.Equal(t, before.BootID, after.BootID)
	require.Equal(t, before.InitializedAt, after.InitializedAt)
	require.Equal(t, fingerprint(next.cert.Raw), after.LeafSHA256)
	result = handshake(t, source.ServingConfig(nil), client)
	require.NoError(t, result.clientErr)
	require.Equal(t, next.cert.Raw, result.clientState.PeerCertificates[0].Raw)

	wrong := ca.issue(t, certOptions{dnsNames: componentSANs("controller")})
	dir.update(t, wrong, ca.pem)
	_, err = proof.Snapshot()
	require.Error(t, err)
	_, err = source.load()
	require.Error(t, err, "invalid renewal must invalidate the actual serving Source")
	recorder := httptest.NewRecorder()
	proof.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, AggregationProofPath, nil))
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	require.JSONEq(t, `{"schemaVersion":1,"ready":false}`, recorder.Body.String())
	require.NotContains(t, recorder.Body.String(), dir.root)

	dir.update(t, next, ca.pem)
	recorder = httptest.NewRecorder()
	proof.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, AggregationProofPath, nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	var evidence AggregationEvidence
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &evidence))
	require.NoError(t, evidence.Validate(uid, digest, fingerprint(next.cert.Raw), time.Now()))
	recorder = httptest.NewRecorder()
	proof.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, AggregationProofPath, nil))
	require.Equal(t, http.StatusMethodNotAllowed, recorder.Code)
	require.Equal(t, "GET", recorder.Header().Get("Allow"))
	other := newCA(t)
	dir.update(t, other.issue(t, certOptions{dnsNames: componentSANs("ucp")}), other.pem)
	_, err = proof.Snapshot()
	require.Error(t, err, "CA change needs a newly pinned process")
}

func TestAggregationProofConfigurationIsolation(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	dir := newKubeletDir(t, ca.issue(t, certOptions{dnsNames: componentSANs("ucp")}), ca.pem)
	source, err := NewSource(dir.root)
	require.NoError(t, err)
	digest, _, err := TrustBundleDigest(ca.pem, time.Now())
	require.NoError(t, err)
	_, err = NewAggregationProof(source, testNamespace, "not-a-uid", digest)
	require.Error(t, err)
	require.Empty(t, source.aggregationNamespace)
	require.Error(t, source.bindProofNamespace(testNamespace))
	trustOnly := newTrustSource(t, ca.pem)
	_, err = NewAggregationProof(trustOnly, testNamespace, uuid.NewString(), digest)
	require.Error(t, err, "outbound authority cannot stand in for serving Source")
	_, err = NewAggregationProof(nil, testNamespace, uuid.NewString(), digest)
	require.Error(t, err)
}

func TestAggregationProofRejectsWrongIdentity(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		options certOptions
	}{
		{"client only", certOptions{dnsNames: componentSANs("ucp"), extKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}},
		{"another service", certOptions{dnsNames: componentSANs("controller")}},
		{"extra SAN", certOptions{dnsNames: append(componentSANs("ucp"), "extra.example")}},
		{"expired", certOptions{dnsNames: componentSANs("ucp"), notBefore: time.Now().Add(-2 * time.Hour), notAfter: time.Now().Add(-time.Hour)}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ca := newCA(t)
			dir := newKubeletDir(t, ca.issue(t, tt.options), ca.pem)
			source, err := NewSource(dir.root)
			if err != nil {
				require.Equal(t, "expired", tt.name)
				return
			}
			digest, _, err := TrustBundleDigest(ca.pem, time.Now())
			require.NoError(t, err)
			_, err = NewAggregationProof(source, testNamespace, uuid.NewString(), digest)
			require.Error(t, err)
		})
	}
}
