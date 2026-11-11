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

package api

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/ucp"
)

// writeServingCert writes a CA and a leaf certificate for ucp.radius-system.svc to dir as tls.crt, tls.key, and
// ca.crt, the files of a cert-manager Secret. It returns the leaf certificate.
func writeServingCert(t *testing.T, dir string) *x509.Certificate {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "radius-apiserver-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"ucp.radius-system.svc"},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caTemplate, &key.PublicKey, caKey)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	write := func(name, blockType string, der []byte) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), 0o600))
	}
	write("tls.crt", "CERTIFICATE", leafDER)
	write("tls.key", "EC PRIVATE KEY", keyDER)
	write("ca.crt", "CERTIFICATE", caDER)

	leaf, err := x509.ParseCertificate(leafDER)
	require.NoError(t, err)
	return leaf
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func newTestService(dir string, mode authz.Mode) *Service {
	return &Service{options: &ucp.Options{Config: &ucp.Config{
		Authorization: authz.Options{Mode: mode},
		Server:        hostoptions.ServerOptions{TLSCertificateDirectory: dir},
	}}}
}

func Test_listenAndServe_ServesCertificateFromDirectory(t *testing.T) {
	for _, mode := range []authz.Mode{authz.ModeOff, authz.ModeDryRun, authz.ModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			dir := t.TempDir()
			leaf := writeServingCert(t, dir)

			addr := "127.0.0.1:" + strconv.Itoa(freePort(t))
			server := &http.Server{Addr: addr, Handler: http.NotFoundHandler(), ReadHeaderTimeout: time.Second}
			errs := make(chan error, 1)
			go func() { errs <- newTestService(dir, mode).listenAndServe(t.Context(), server) }()
			t.Cleanup(func() {
				_ = server.Close()
				require.ErrorIs(t, <-errs, http.ErrServerClosed)
			})

			var state tls.ConnectionState
			require.Eventually(t, func() bool {
				conn, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}) //nolint:gosec // The test checks which certificate is served.
				if err != nil {
					return false
				}
				defer conn.Close()
				state = conn.ConnectionState()
				return true
			}, 5*time.Second, 10*time.Millisecond)
			require.Equal(t, leaf.Raw, state.PeerCertificates[0].Raw)
		})
	}
}

func Test_listenAndServe_FailsWithoutCertificateWhenAuthorizationIsOn(t *testing.T) {
	err := newTestService(t.TempDir(), authz.ModeEnforce).listenAndServe(t.Context(), &http.Server{Addr: "127.0.0.1:0", ReadHeaderTimeout: time.Second})
	require.ErrorContains(t, err, "failed to load the API server certificate")
}

// newProxyClientCert returns a CA pool and a client certificate it signs, like the Kubernetes
// API server's front-proxy client certificate.
func newProxyClientCert(t *testing.T) (*x509.CertPool, *x509.Certificate, tls.Certificate) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(10),
		Subject:               pkix.Name{CommonName: "front-proxy-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	ca, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	leafDER, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(11),
		Subject:      pkix.Name{CommonName: "front-proxy-client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}, ca, &key.PublicKey, caKey)
	require.NoError(t, err)

	pool := x509.NewCertPool()
	pool.AddCert(ca)
	return pool, ca, tls.Certificate{Certificate: [][]byte{leafDER}, PrivateKey: key}
}

// verifyProxyCertificate applies the request-header verification contract: with
// RequestClientCert, TLS does not verify the presented certificate, so the application verifies
// it against the proxy roots and requires the clientAuth usage.
func verifyProxyCertificate(r *http.Request, roots *x509.CertPool) error {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return errors.New("no client certificate")
	}
	intermediates := x509.NewCertPool()
	for _, cert := range r.TLS.PeerCertificates[1:] {
		intermediates.AddCert(cert)
	}
	_, err := r.TLS.PeerCertificates[0].Verify(x509.VerifyOptions{
		Roots: roots, Intermediates: intermediates, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	return err
}

// Test_listenAndServe_PreservesRequestHeaderClientAuth checks that the rotating certificate does
// not replace the request-header proxy's client-certificate settings on the aggregated API server.
func Test_listenAndServe_PreservesRequestHeaderClientAuth(t *testing.T) {
	for _, mode := range []authz.Mode{authz.ModeOff, authz.ModeDryRun, authz.ModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			dir := t.TempDir()
			leaf := writeServingCert(t, dir)
			roots, proxyCA, proxyCert := newProxyClientCert(t)
			_, _, untrustedCert := newProxyClientCert(t)

			addr := "127.0.0.1:" + strconv.Itoa(freePort(t))
			base := &tls.Config{ClientAuth: tls.RequestClientCert, ClientCAs: roots}
			server := &http.Server{
				Addr:              addr,
				TLSConfig:         base,
				ReadHeaderTimeout: time.Second,
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// TLS leaves VerifiedChains empty for RequestClientCert.
					if len(r.TLS.VerifiedChains) != 0 {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					if err := verifyProxyCertificate(r, roots); err != nil {
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					w.WriteHeader(http.StatusOK)
				}),
			}
			errs := make(chan error, 1)
			go func() { errs <- newTestService(dir, mode).listenAndServe(t.Context(), server) }()
			t.Cleanup(func() {
				_ = server.Close()
				require.ErrorIs(t, <-errs, http.ErrServerClosed)
			})

			// newClient presents cert, if any, and records the CAs the server advertised.
			newClient := func(cert *tls.Certificate, acceptable *[][]byte) *http.Client {
				return &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true, //nolint:gosec // The test checks client-certificate negotiation; VerifyConnection checks the served certificate.
					MinVersion:         tls.VersionTLS12,
					GetClientCertificate: func(info *tls.CertificateRequestInfo) (*tls.Certificate, error) {
						*acceptable = info.AcceptableCAs
						if cert == nil {
							return &tls.Certificate{}, nil
						}
						return cert, nil
					},
					VerifyConnection: func(cs tls.ConnectionState) error {
						if len(cs.PeerCertificates) == 0 || string(cs.PeerCertificates[0].Raw) != string(leaf.Raw) {
							return errors.New("server did not present the certificate from the directory")
						}
						return nil
					},
				}}}
			}
			status := func(client *http.Client) int {
				var code int
				require.Eventually(t, func() bool {
					resp, err := client.Get("https://" + addr + "/")
					if err != nil {
						return false
					}
					_ = resp.Body.Close()
					code = resp.StatusCode
					return true
				}, 5*time.Second, 10*time.Millisecond)
				return code
			}

			tests := []struct {
				name string
				cert *tls.Certificate
				want int
			}{
				{name: "trusted proxy certificate is verified", cert: &proxyCert, want: http.StatusOK},
				{name: "untrusted certificate is rejected by the application", cert: &untrustedCert, want: http.StatusUnauthorized},
				{name: "caller without a certificate still connects", want: http.StatusUnauthorized},
			}
			for _, tt := range tests {
				var acceptable [][]byte
				require.Equal(t, tt.want, status(newClient(tt.cert, &acceptable)), tt.name)
				// The server requested a client certificate and advertised the proxy CA from ClientCAs.
				require.Equal(t, [][]byte{proxyCA.RawSubject}, acceptable, tt.name)
			}
			require.Equal(t, tls.RequestClientCert, base.ClientAuth)
			require.Nil(t, base.GetConfigForClient)
		})
	}
}
