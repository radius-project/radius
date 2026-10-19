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
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const testNamespace = "radius-system"

type testCA struct {
	cert *x509.Certificate
	key  crypto.Signer
	pem  []byte
}

type testCert struct {
	cert    *x509.Certificate
	certPEM []byte
	keyPEM  []byte
}

// tlsCertificate returns the certificate and key as a tls.Certificate.
func (c testCert) tlsCertificate(t *testing.T) tls.Certificate {
	t.Helper()
	cert, err := tls.X509KeyPair(c.certPEM, c.keyPEM)
	require.NoError(t, err)
	return cert
}

type certOptions struct {
	dnsNames  []string
	cn        string
	key       crypto.Signer
	notBefore time.Time
	notAfter  time.Time
}

func ecKey(t *testing.T, curve elliptic.Curve) crypto.Signer {
	t.Helper()
	key, err := ecdsa.GenerateKey(curve, rand.Reader)
	require.NoError(t, err)
	return key
}

func rsaKey(t *testing.T, bits int) crypto.Signer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, bits) //nolint:gosec // weak keys are generated to test rejection.
	require.NoError(t, err)
	return key
}

func newCA(t *testing.T) testCA {
	t.Helper()
	key := ecKey(t, elliptic.P256())
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "radius-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return testCA{cert: cert, key: key, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

func (ca testCA) issue(t *testing.T, opts certOptions) testCert {
	t.Helper()
	if opts.key == nil {
		opts.key = ecKey(t, elliptic.P256())
	}
	if opts.notBefore.IsZero() {
		opts.notBefore = time.Now().Add(-time.Hour)
	}
	if opts.notAfter.IsZero() {
		opts.notAfter = time.Now().Add(time.Hour)
	}

	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: opts.cn},
		DNSNames:     opts.dnsNames,
		NotBefore:    opts.notBefore,
		NotAfter:     opts.notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, opts.key.Public(), ca.key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(opts.key)
	require.NoError(t, err)

	return testCert{
		cert:    cert,
		certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		keyPEM:  pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
	}
}

// componentSANs returns the DNS SANs the Helm chart issues for component.
func componentSANs(component string) []string {
	return []string{component, component + "." + testNamespace, component + "." + testNamespace + ".svc"}
}

func writeFiles(t *testing.T, dir string, cert testCert, caPEM []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, CertFile), cert.certPEM, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, KeyFile), cert.keyPEM, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, CAFile), caPEM, 0o600))
}

// newTestSource returns a Source for component with a certificate from ca.
func newTestSource(t *testing.T, ca testCA, component string) *Source {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, ca.issue(t, certOptions{dnsNames: componentSANs(component)}), ca.pem)
	src, err := NewSource(dir)
	require.NoError(t, err)
	return src
}

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	require.Error(t, err)
	var certErr *Error
	require.ErrorAs(t, err, &certErr)
	require.Equal(t, code, certErr.Code)
}

func TestPeerIdentity(t *testing.T) {
	t.Parallel()
	ca := newCA(t)

	tests := []struct {
		name     string
		opts     certOptions
		expected string
	}{
		{name: "chart SANs", opts: certOptions{dnsNames: componentSANs("applications-rp")}, expected: "applications-rp.radius-system.svc"},
		{name: "canonical SAN only", opts: certOptions{dnsNames: []string{"bicep-de.radius-system.svc"}}, expected: "bicep-de.radius-system.svc"},
		{name: "CN ignored", opts: certOptions{cn: "dynamic-rp", dnsNames: []string{"ucp"}}, expected: "ucp.radius-system.svc"},
		{name: "RSA-2048 accepted", opts: certOptions{dnsNames: []string{"ucp"}, key: rsaKey(t, 2048)}, expected: "ucp.radius-system.svc"},
		{name: "P-384 accepted", opts: certOptions{dnsNames: []string{"ucp"}, key: ecKey(t, elliptic.P384())}, expected: "ucp.radius-system.svc"},
		{name: "no SAN", opts: certOptions{cn: "ucp"}},
		{name: "two components", opts: certOptions{dnsNames: []string{"ucp", "dynamic-rp.radius-system.svc"}}},
		{name: "SAN outside list", opts: certOptions{dnsNames: []string{"ucp", "evil.radius-system.svc"}}},
		{name: "wrong namespace", opts: certOptions{dnsNames: []string{"ucp.other.svc"}}},
		{name: "extra DNS labels", opts: certOptions{dnsNames: []string{"ucp.radius-system.svc.cluster.local"}}},
		{name: "RSA-1024 rejected", opts: certOptions{dnsNames: []string{"ucp"}, key: rsaKey(t, 1024)}},
		{name: "P-224 rejected", opts: certOptions{dnsNames: []string{"ucp"}, key: ecKey(t, elliptic.P224())}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cert := ca.issue(t, tt.opts)
			id, err := PeerIdentity([]*x509.Certificate{cert.cert, ca.cert}, testNamespace)
			if tt.expected == "" {
				requireCode(t, err, "PeerCertificateInvalid")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, id.String())
		})
	}
}

func TestPeerIdentity_Ed25519Rejected(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	cert := ca.issue(t, certOptions{dnsNames: []string{"ucp"}, key: key})
	_, err = PeerIdentity([]*x509.Certificate{cert.cert}, testNamespace)
	requireCode(t, err, "PeerCertificateInvalid")
}

func TestPeerIdentity_NoCertificate(t *testing.T) {
	t.Parallel()
	_, err := PeerIdentity(nil, testNamespace)
	requireCode(t, err, "PeerCertificateInvalid")
}
