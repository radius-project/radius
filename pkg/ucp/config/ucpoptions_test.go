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

// writeCertificate writes a self-signed controller certificate, key, and CA to a new directory.
// It also returns the certificate so a test server can present it. The certificate is valid
// for 127.0.0.1 so that only the CA decides whether a client trusts that server.
func writeCertificate(t *testing.T) (string, tls.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "controller"},
		DNSNames:              []string{"controller.radius-system.svc"},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
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
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)
	return dir, cert
}

func Test_NewConnectionFromUCPConfig(t *testing.T) {
	certDir, _ := writeCertificate(t)
	on := func(mode authz.Mode) authz.Options {
		return authz.Options{Mode: mode, CertificateDirectory: certDir, Namespace: "radius-system"}
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
		{name: "enforce missing certificate directory", option: direct("https://ucp.radius-system.svc:9444"), authorization: authz.Options{Mode: authz.ModeEnforce, Namespace: "radius-system"}, wantErr: "requires certificateDirectory and namespace"},
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
// by calling a TLS server at 127.0.0.1 whose certificate comes from the Radius CA and is valid
// for that address. The default transport rejects the unknown CA; the mTLS client trusts the CA
// but rejects 127.0.0.1 because it is not a Radius component name.
func Test_NewConnectionFromUCPConfig_Transport(t *testing.T) {
	certDir, cert := writeCertificate(t)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	tests := []struct {
		mode    authz.Mode
		wantErr string
	}{
		{mode: authz.ModeOff, wantErr: "x509: certificate signed by unknown authority"},
		{mode: authz.ModeDryRun, wantErr: authz.CodePeerCertificateInvalid},
		{mode: authz.ModeEnforce, wantErr: authz.CodePeerCertificateInvalid},
	}
	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			options := authz.Options{Mode: tt.mode, CertificateDirectory: certDir, Namespace: "radius-system"}
			conn, err := NewConnectionFromUCPConfig(&UCPOptions{Kind: UCPConnectionKindDirect, Direct: &UCPDirectConnectionOptions{Endpoint: srv.URL}}, nil, options)
			require.NoError(t, err)
			require.IsType(t, &otelhttp.Transport{}, conn.Client().Transport)

			resp, err := conn.Client().Get(srv.URL)
			if err == nil {
				_ = resp.Body.Close()
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}
