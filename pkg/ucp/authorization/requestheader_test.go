// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/stretchr/testify/require"
)

type failingAuthenticationWriter struct {
	header http.Header
	status int
	body   []byte
	err    error
}

func (w *failingAuthenticationWriter) Header() http.Header    { return w.header }
func (w *failingAuthenticationWriter) WriteHeader(status int) { w.status = status }
func (w *failingAuthenticationWriter) Write(body []byte) (int, error) {
	w.body = append(w.body, body...)
	return 0, w.err
}

func TestRequestHeaderAuthenticationResponseWriteFailure(t *testing.T) {
	t.Parallel()
	authenticator, err := NewRequestHeaderAuthenticator(nil)
	require.NoError(t, err)
	var logs []string
	logger := funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-Remote-User", "forged")
	request = request.WithContext(logr.NewContext(request.Context(), logger))
	writer := &failingAuthenticationWriter{
		header: http.Header{}, err: errors.New("response connection closed"),
	}
	called := false
	authenticator.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	})).ServeHTTP(writer, request)
	require.False(t, called, "a response write failure must not admit the rejected request")
	require.Equal(t, http.StatusUnauthorized, writer.status)
	require.Equal(t, "application/json", writer.header.Get("Content-Type"))
	var body v1.ErrorResponse
	require.NoError(t, json.Unmarshal(writer.body, &body))
	require.Equal(t, v1.CodeInvalidAuthenticationInfo, body.Error.Code)
	require.NotEmpty(t, logs)
	require.Contains(t, logs[len(logs)-1], "failed to write authentication error response")
	require.Contains(t, logs[len(logs)-1], writer.err.Error())
	_, ok := IdentityFromContext(request.Context())
	require.False(t, ok)
}

type proxyPKI struct {
	ca   *x509.Certificate
	key  *ecdsa.PrivateKey
	file string
}

func newProxyPKI(t *testing.T) proxyPKI {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "request-header-ca"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	ca, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	file := filepath.Join(t.TempDir(), "request-header-ca.crt")
	require.NoError(t, os.WriteFile(file, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600))
	return proxyPKI{ca: ca, key: key, file: file}
}

func (p proxyPKI) client(t *testing.T, name string, usage x509.ExtKeyUsage, expired bool) *x509.Certificate {
	t.Helper()
	return p.clientWithUsages(t, name, []x509.ExtKeyUsage{usage}, expired)
}

func (p proxyPKI) clientWithUsages(t *testing.T, name string, usages []x509.ExtKeyUsage, expired bool) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	end := time.Now().Add(time.Hour)
	if expired {
		end = time.Now().Add(-time.Minute)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: end,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: usages,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, p.ca, &key.PublicKey, p.key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return cert
}

func (p proxyPKI) authenticator(t *testing.T) *RequestHeaderAuthenticator {
	t.Helper()
	authenticator, err := NewRequestHeaderAuthenticator(&RequestHeaderOptions{
		CAFile: p.file, AllowedNames: []string{"front-proxy-client"}, KubernetesIssuer: "kubernetes:test-cluster",
	})
	require.NoError(t, err)
	return authenticator
}

func TestRequestHeaderCertificateTrust(t *testing.T) {
	t.Parallel()
	pki := newProxyPKI(t)
	other := newProxyPKI(t)
	intermediate := newProxyPKI(t)
	template := *intermediate.ca
	template.Subject.CommonName = "request-header-intermediate"
	der, err := x509.CreateCertificate(rand.Reader, &template, pki.ca, &intermediate.key.PublicKey, pki.key)
	require.NoError(t, err)
	intermediate.ca, err = x509.ParseCertificate(der)
	require.NoError(t, err)
	intermediateClient := intermediate.client(t, "front-proxy-client", x509.ExtKeyUsageClientAuth, false)
	authenticator := pki.authenticator(t)
	good := pki.client(t, "front-proxy-client", x509.ExtKeyUsageClientAuth, false)
	tests := []struct {
		name  string
		state *tls.ConnectionState
		valid bool
	}{
		{name: "plain HTTP"},
		{name: "missing client certificate", state: &tls.ConnectionState{}},
		{name: "valid proxy", state: &tls.ConnectionState{PeerCertificates: []*x509.Certificate{good}}, valid: true},
		{name: "valid intermediate chain", state: &tls.ConnectionState{PeerCertificates: []*x509.Certificate{
			intermediateClient, intermediate.ca,
		}}, valid: true},
		{name: "missing intermediate", state: &tls.ConnectionState{PeerCertificates: []*x509.Certificate{intermediateClient}}},
		{name: "untrusted CA", state: &tls.ConnectionState{PeerCertificates: []*x509.Certificate{
			other.client(t, "front-proxy-client", x509.ExtKeyUsageClientAuth, false), other.ca,
		}}},
		{name: "wrong name", state: &tls.ConnectionState{PeerCertificates: []*x509.Certificate{
			pki.client(t, "other-client", x509.ExtKeyUsageClientAuth, false),
		}}},
		{name: "names are case sensitive", state: &tls.ConnectionState{PeerCertificates: []*x509.Certificate{
			pki.client(t, "FRONT-PROXY-CLIENT", x509.ExtKeyUsageClientAuth, false),
		}}},
		{name: "server certificate is not a client", state: &tls.ConnectionState{PeerCertificates: []*x509.Certificate{
			pki.client(t, "front-proxy-client", x509.ExtKeyUsageServerAuth, false),
		}}},
		{name: "missing EKU is not explicit client authentication", state: &tls.ConnectionState{PeerCertificates: []*x509.Certificate{
			pki.clientWithUsages(t, "front-proxy-client", nil, false),
		}}},
		{name: "any EKU is not explicit client authentication", state: &tls.ConnectionState{PeerCertificates: []*x509.Certificate{
			pki.client(t, "front-proxy-client", x509.ExtKeyUsageAny, false),
		}}},
		{name: "explicit client authentication with other usages", state: &tls.ConnectionState{PeerCertificates: []*x509.Certificate{
			pki.clientWithUsages(t, "front-proxy-client", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}, false),
		}}, valid: true},
		{name: "expired certificate", state: &tls.ConnectionState{PeerCertificates: []*x509.Certificate{
			pki.client(t, "front-proxy-client", x509.ExtKeyUsageClientAuth, true),
		}}},
		{name: "other TLS verifier cannot assert proxy trust", state: &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{other.client(t, "front-proxy-client", x509.ExtKeyUsageClientAuth, false)},
			VerifiedChains:   [][]*x509.Certificate{{good, pki.ca}},
		}},
		{name: "verified chains alone are not a peer", state: &tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{good, pki.ca}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.TLS = tt.state
			r.Header.Set("X-Remote-User", "alice")
			r.Header.Set("X-SSL-Client-Cert", "not a TLS peer certificate")
			identity, err := authenticator.Authenticate(r)
			if !tt.valid {
				require.Error(t, err)
				require.Empty(t, identity)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "alice", *identity.Principal.Subject)
		})
	}
}

func TestRequestHeaderIdentityMapping(t *testing.T) {
	t.Parallel()
	pki := newProxyPKI(t)
	authenticator := pki.authenticator(t)
	cert := pki.client(t, "front-proxy-client", x509.ExtKeyUsageClientAuth, false)
	tests := []struct {
		name    string
		headers http.Header
		typ     api.PrincipalType
		valid   bool
	}{
		{name: "user and repeated groups", headers: http.Header{
			"X-Remote-User": {"Alice@EXAMPLE.com"}, "X-Remote-Group": {"team-a", "system:authenticated", "team,b"},
		}, typ: api.PrincipalTypeUser, valid: true},
		{name: "workload", headers: http.Header{
			"X-Remote-User":  {"system:serviceaccount:team-a:ci"},
			"X-Remote-Group": {"system:serviceaccounts", "system:serviceaccounts:team-a"},
		}, typ: api.PrincipalTypeWorkload, valid: true},
		{name: "user without groups", headers: http.Header{"X-Remote-User": {"alice"}}, typ: api.PrincipalTypeUser, valid: true},
		{name: "missing user", headers: http.Header{"X-Remote-Group": {"admins"}}},
		{name: "empty user", headers: http.Header{"X-Remote-User": {""}}},
		{name: "whitespace user", headers: http.Header{"X-Remote-User": {"  "}}},
		{name: "duplicate user", headers: http.Header{"X-Remote-User": {"alice", "admin"}}},
		{name: "case variant duplicate user", headers: http.Header{"X-Remote-User": {"alice"}, "x-remote-user": {"admin"}}},
		{name: "anonymous", headers: http.Header{"X-Remote-User": {"system:anonymous"}}},
		{name: "unauthenticated user", headers: http.Header{"X-Remote-User": {"system:unauthenticated"}}},
		{name: "unauthenticated group", headers: http.Header{"X-Remote-User": {"alice"}, "X-Remote-Group": {"system:unauthenticated"}}},
		{name: "anonymous group", headers: http.Header{"X-Remote-User": {"alice"}, "X-Remote-Group": {"system:anonymous"}}},
		{name: "empty group", headers: http.Header{"X-Remote-User": {"alice"}, "X-Remote-Group": {""}}},
		{name: "whitespace group", headers: http.Header{"X-Remote-User": {"alice"}, "X-Remote-Group": {" "}}},
		{name: "missing service account namespace", headers: http.Header{"X-Remote-User": {"system:serviceaccount::ci"}}},
		{name: "missing service account name", headers: http.Header{"X-Remote-User": {"system:serviceaccount:team-a:"}}},
		{name: "extra service account segment", headers: http.Header{"X-Remote-User": {"system:serviceaccount:team-a:ci:other"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}
			r.Header = tt.headers
			identity, err := authenticator.Authenticate(r)
			if !tt.valid {
				require.Error(t, err)
				require.Empty(t, identity)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.typ, *identity.Principal.Type)
			require.Equal(t, "kubernetes:test-cluster", *identity.Principal.Issuer)
			require.Equal(t, tt.headers.Get("X-Remote-User"), *identity.Principal.Subject)
			require.Len(t, identity.Groups, len(tt.headers.Values("X-Remote-Group")))
			for i, group := range identity.Groups {
				require.Equal(t, api.PrincipalTypeGroup, *group.Type)
				require.Equal(t, "kubernetes:test-cluster", *group.Issuer)
				require.Equal(t, tt.headers.Values("X-Remote-Group")[i], *group.Subject)
			}
		})
	}
}

func TestRequestHeaderMiddleware(t *testing.T) {
	t.Parallel()
	pki := newProxyPKI(t)
	cert := pki.client(t, "front-proxy-client", x509.ExtKeyUsageClientAuth, false)
	for _, configured := range []bool{false, true} {
		name := "unconfigured"
		authenticator, err := NewRequestHeaderAuthenticator(nil)
		require.NoError(t, err)
		if configured {
			name = "configured"
			authenticator = pki.authenticator(t)
		}
		t.Run(name, func(t *testing.T) {
			for _, header := range []string{"X-Remote-User", "X-Remote-Group", "X-Remote-Extra-Admin", "x-remote-uid"} {
				t.Run("reject spoofed "+header, func(t *testing.T) {
					r := httptest.NewRequest(http.MethodGet, "/version", nil)
					r.Header[header] = []string{"admin"}
					response := httptest.NewRecorder()
					authenticator.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
						t.Fatal("spoofed headers reached handler")
					})).ServeHTTP(response, r)
					require.Equal(t, http.StatusUnauthorized, response.Code)
					var body v1.ErrorResponse
					require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
					require.Equal(t, v1.CodeInvalidAuthenticationInfo, body.Error.Code)
				})
			}
			t.Run("headerless is unidentified", func(t *testing.T) {
				r := httptest.NewRequest(http.MethodGet, "/", nil)
				response := httptest.NewRecorder()
				authenticator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, ok := IdentityFromContext(r.Context())
					require.False(t, ok)
					w.WriteHeader(http.StatusNoContent)
				})).ServeHTTP(response, r)
				require.Equal(t, http.StatusNoContent, response.Code)
			})
			t.Run("verified identity requires configuration", func(t *testing.T) {
				r := httptest.NewRequest(http.MethodGet, "/", nil)
				r.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}
				r.Header.Set("X-Remote-User", "alice")
				r.Header.Set("X-Remote-Group", "team-a")
				r.Header.Set("X-Remote-Extra-Admin", "true")
				r.Header.Set("X-Remote-Uid", "uid")
				r.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				authenticator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, received *http.Request) {
					require.True(t, configured)
					identity, ok := IdentityFromContext(received.Context())
					require.True(t, ok)
					require.Equal(t, "alice", *identity.Principal.Subject)
					require.Equal(t, http.Header{"Content-Type": {"application/json"}}, received.Header)
					// A consumer must not be able to mutate the authenticator's issuer.
					*identity.Principal.Issuer = "modified"
					w.WriteHeader(http.StatusNoContent)
				})).ServeHTTP(response, r)
				if configured {
					require.Equal(t, http.StatusNoContent, response.Code)
					require.Equal(t, "kubernetes:test-cluster", authenticator.issuer)
					require.Equal(t, "alice", r.Header.Get("X-Remote-User"))
					_, ok := IdentityFromContext(r.Context())
					require.False(t, ok)
				} else {
					require.Equal(t, http.StatusUnauthorized, response.Code)
				}
			})
		})
	}
}

func TestRequestHeaderTrailerAnnouncements(t *testing.T) {
	t.Parallel()
	pki := newProxyPKI(t)
	authenticator := pki.authenticator(t)
	cert := pki.client(t, "front-proxy-client", x509.ExtKeyUsageClientAuth, false)
	for _, name := range []string{"x-remote-user", "X-REMOTE-GROUP", "x-ReMoTe-ExTrA-Admin", "X-Remote-Uid"} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", nil)
			r.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}
			r.Header.Set("X-Remote-User", "alice")
			r.Trailer = http.Header{name: nil}
			identity, err := authenticator.Authenticate(r)
			require.ErrorContains(t, err, "identity must not be supplied in trailers")
			require.Empty(t, identity)
			response := httptest.NewRecorder()
			authenticator.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Error("announced identity trailer reached handler")
			})).ServeHTTP(response, r)
			require.Equal(t, http.StatusUnauthorized, response.Code)
		})
	}
}

func TestRequestHeaderConfiguration(t *testing.T) {
	t.Parallel()
	pki := newProxyPKI(t)
	empty := filepath.Join(t.TempDir(), "empty.crt")
	require.NoError(t, os.WriteFile(empty, []byte("not a PEM certificate"), 0600))
	for _, tt := range []struct {
		name    string
		options *RequestHeaderOptions
		valid   bool
	}{
		{name: "unconfigured", valid: true},
		{name: "empty configuration", options: &RequestHeaderOptions{}},
		{name: "unqualified issuer", options: &RequestHeaderOptions{CAFile: pki.file, KubernetesIssuer: "kubernetes"}},
		{name: "empty trust domain", options: &RequestHeaderOptions{CAFile: pki.file, KubernetesIssuer: "kubernetes:"}},
		{name: "whitespace trust domain", options: &RequestHeaderOptions{CAFile: pki.file, KubernetesIssuer: "kubernetes: "}},
		{name: "other issuer", options: &RequestHeaderOptions{CAFile: pki.file, KubernetesIssuer: "oidc:cluster"}},
		{name: "missing CA", options: &RequestHeaderOptions{CAFile: "does-not-exist", KubernetesIssuer: "kubernetes:cluster"}},
		{name: "invalid CA", options: &RequestHeaderOptions{CAFile: empty, KubernetesIssuer: "kubernetes:cluster"}},
		{name: "empty allowed name", options: &RequestHeaderOptions{CAFile: pki.file, KubernetesIssuer: "kubernetes:cluster", AllowedNames: []string{""}}},
		{name: "Kubernetes empty allowed names", options: &RequestHeaderOptions{CAFile: pki.file, KubernetesIssuer: "kubernetes:cluster"}, valid: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			authenticator, err := NewRequestHeaderAuthenticator(tt.options)
			if !tt.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.options == nil {
				require.Nil(t, authenticator.TLSConfig())
				return
			}
			require.Equal(t, tls.RequestClientCert, authenticator.TLSConfig().ClientAuth)
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{pki.client(t, "any-proxy-name", x509.ExtKeyUsageClientAuth, false)}}
			r.Header.Set("X-Remote-User", "alice")
			_, err = authenticator.Authenticate(r)
			require.NoError(t, err)
		})
	}
}

func TestRequestHeaderTrustSnapshot(t *testing.T) {
	t.Parallel()
	pki := newProxyPKI(t)
	replacement := newProxyPKI(t)
	options := &RequestHeaderOptions{
		CAFile: pki.file, AllowedNames: []string{"front-proxy-client"}, KubernetesIssuer: "kubernetes:cluster",
	}
	old, err := NewRequestHeaderAuthenticator(options)
	require.NoError(t, err)
	// Replacing a mounted bundle takes effect only after constructing a new authenticator.
	bundle, err := os.ReadFile(replacement.file)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(pki.file, bundle, 0600))
	fresh, err := NewRequestHeaderAuthenticator(options)
	require.NoError(t, err)
	options.AllowedNames[0] = "modified-after-startup"
	for _, tt := range []struct {
		name  string
		auth  *RequestHeaderAuthenticator
		pki   proxyPKI
		valid bool
	}{
		{name: "old snapshot accepts old CA", auth: old, pki: pki, valid: true},
		{name: "old snapshot rejects new CA", auth: old, pki: replacement},
		{name: "new snapshot rejects old CA", auth: fresh, pki: pki},
		{name: "new snapshot accepts new CA", auth: fresh, pki: replacement, valid: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{
				tt.pki.client(t, "front-proxy-client", x509.ExtKeyUsageClientAuth, false),
			}}
			r.Header.Set("X-Remote-User", "alice")
			_, err := tt.auth.Authenticate(r)
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
