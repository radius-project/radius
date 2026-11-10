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
