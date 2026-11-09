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

package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"k8s.io/client-go/rest"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/mtls"
)

const testToken = "test-token"

// writeCertificate writes a self-signed certificate, key, and CA to a new directory, and a
// projected token for UCP to a new token directory. The certificate carries the SPIFFE ID
// of serviceAccount and is valid for 127.0.0.1, so only the CA and SPIFFE ID decide whether
// a client trusts a server that presents it.
func writeCertificate(t *testing.T, serviceAccount string) (string, string, tls.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	spiffeID, err := url.Parse(mtls.SPIFFEID(mtls.DefaultTrustDomain, "radius-system", serviceAccount))
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: serviceAccount},
		URIs:                  []*url.URL{spiffeID},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, mtls.CertFile), certPEM, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, mtls.KeyFile), keyPEM, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, mtls.CAFile), certPEM, 0o600))

	tokenDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tokenDir, "ucp"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(tokenDir, "ucp", mtls.TokenFile), []byte(testToken), 0o600))

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)
	return dir, tokenDir, cert
}

func Test_NewConnectionFromUCPConfig(t *testing.T) {
	certDir, tokenDir, _ := writeCertificate(t, "ucp")
	on := func(mode authz.Mode) authz.Options {
		return authz.Options{Mode: mode, CertificateDirectory: certDir, TokenDirectory: tokenDir, Namespace: "radius-system"}
	}
	direct := func(endpoint string) *UCPOptions {
		return &UCPOptions{Kind: UCPConnectionKindDirect, Direct: &UCPDirectConnectionOptions{Endpoint: endpoint}}
	}

	tests := []struct {
		name          string
		option        *UCPOptions
		authorization authz.Options
		wantEndpoint  string
		wantErr       string
	}{
		{name: "off direct http", option: direct("http://localhost:9000/apis/api.ucp.dev/v1alpha3"), wantEndpoint: "http://localhost:9000/apis/api.ucp.dev/v1alpha3"},
		{name: "off kubernetes", option: &UCPOptions{Kind: UCPConnectionKindKubernetes}, wantEndpoint: "https://kube:6443/apis/api.ucp.dev/v1alpha3"},
		{name: "dryRun direct https", option: direct("https://ucp.radius-system.svc:9444/apis/api.ucp.dev/v1alpha3"), authorization: on(authz.ModeDryRun), wantEndpoint: "https://ucp.radius-system.svc:9444/apis/api.ucp.dev/v1alpha3"},
		{name: "enforce direct https", option: direct("https://ucp.radius-system.svc:9444/apis/api.ucp.dev/v1alpha3"), authorization: on(authz.ModeEnforce), wantEndpoint: "https://ucp.radius-system.svc:9444/apis/api.ucp.dev/v1alpha3"},
		{name: "enforce kubernetes", option: &UCPOptions{Kind: UCPConnectionKindKubernetes}, authorization: on(authz.ModeEnforce), wantEndpoint: "https://kube:6443/apis/api.ucp.dev/v1alpha3"},
		{name: "enforce direct http", option: direct("http://ucp.radius-system.svc:9444"), authorization: on(authz.ModeEnforce), wantErr: "must use https when the authorization mode is \"enforce\""},
		{name: "dryRun direct http", option: direct("http://ucp.radius-system.svc:9444"), authorization: on(authz.ModeDryRun), wantErr: "must use https when the authorization mode is \"dryRun\""},
		{name: "enforce missing certificate directory", option: direct("https://ucp.radius-system.svc:9444"), authorization: authz.Options{Mode: authz.ModeEnforce, TokenDirectory: tokenDir, Namespace: "radius-system"}, wantErr: "requires certificateDirectory, tokenDirectory, and namespace"},
		{name: "enforce missing token directory", option: direct("https://ucp.radius-system.svc:9444"), authorization: authz.Options{Mode: authz.ModeEnforce, CertificateDirectory: certDir, Namespace: "radius-system"}, wantErr: "requires certificateDirectory, tokenDirectory, and namespace"},
		{name: "enforce missing token", option: direct("https://ucp.radius-system.svc:9444"), authorization: authz.Options{Mode: authz.ModeEnforce, CertificateDirectory: certDir, TokenDirectory: t.TempDir(), Namespace: "radius-system"}, wantErr: "failed to read ServiceAccount token"},
		{name: "direct missing endpoint", option: &UCPOptions{Kind: UCPConnectionKindDirect}, wantErr: ".ucp.direct.endpoint is required"},
		{name: "invalid kind", option: &UCPOptions{Kind: "other"}, wantErr: "invalid connection kind"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, err := NewConnectionFromUCPConfig(tt.option, &rest.Config{Host: "https://kube:6443"}, tt.authorization)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantEndpoint, conn.Endpoint())
		})
	}
}

// Test_NewConnectionFromUCPConfig_Transport checks which transport a direct connection uses
// by calling a TLS server at 127.0.0.1 whose certificate comes from the Radius CA. The default
// transport rejects the unknown CA. The authenticated transport sends UCP's token to a server
// with UCP's SPIFFE ID, and rejects any other SPIFFE ID without sending the token.
func Test_NewConnectionFromUCPConfig_Transport(t *testing.T) {
	tests := []struct {
		name           string
		mode           authz.Mode
		serviceAccount string
		wantErr        string
		wantAuth       string
	}{
		{name: "off", mode: authz.ModeOff, serviceAccount: "ucp", wantErr: "x509: certificate signed by unknown authority"},
		{name: "dryRun ucp", mode: authz.ModeDryRun, serviceAccount: "ucp", wantAuth: "Bearer " + testToken},
		{name: "enforce ucp", mode: authz.ModeEnforce, serviceAccount: "ucp", wantAuth: "Bearer " + testToken},
		{name: "enforce wrong SPIFFE ID", mode: authz.ModeEnforce, serviceAccount: "controller", wantErr: authz.CodePeerCertificateInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			certDir, tokenDir, cert := writeCertificate(t, tt.serviceAccount)
			var gotAuth []string
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth = append(gotAuth, r.Header.Get("Authorization"))
			}))
			srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
			srv.StartTLS()
			t.Cleanup(srv.Close)

			options := authz.Options{Mode: tt.mode, CertificateDirectory: certDir, TokenDirectory: tokenDir, Namespace: "radius-system"}
			conn, err := NewConnectionFromUCPConfig(&UCPOptions{Kind: UCPConnectionKindDirect, Direct: &UCPDirectConnectionOptions{Endpoint: srv.URL}}, nil, options)
			require.NoError(t, err)
			require.IsType(t, &otelhttp.Transport{}, conn.Client().Transport)

			resp, err := conn.Client().Get(srv.URL)
			if err == nil {
				_ = resp.Body.Close()
			}
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Empty(t, gotAuth)
				return
			}
			require.NoError(t, err)
			require.Equal(t, []string{tt.wantAuth}, gotAuth)
		})
	}
}
