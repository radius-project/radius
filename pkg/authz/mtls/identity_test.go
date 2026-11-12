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
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/authz"
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
	uris        []string
	dnsNames    []string
	ipAddresses []net.IP
	emails      []string
	key         crypto.Signer
	notBefore   time.Time
	notAfter    time.Time
	extKeyUsage []x509.ExtKeyUsage
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

// renew returns a new self-signed certificate for the CA's key, as cert-manager issues when it
// renews a CA Certificate with rotationPolicy Never.
func (ca testCA) renew(t *testing.T) testCA {
	t.Helper()
	template := *ca.cert
	template.SerialNumber = big.NewInt(2)
	template.NotAfter = time.Now().Add(2 * time.Hour)
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, ca.key.Public(), ca.key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return testCA{cert: cert, key: ca.key, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
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
	if opts.extKeyUsage == nil {
		// csi-driver-spiffe requests both usages.
		opts.extKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
	}
	var uris []*url.URL
	for _, u := range opts.uris {
		parsed, err := url.Parse(u)
		require.NoError(t, err)
		uris = append(uris, parsed)
	}

	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber:   serial,
		URIs:           uris,
		DNSNames:       opts.dnsNames,
		IPAddresses:    opts.ipAddresses,
		EmailAddresses: opts.emails,
		NotBefore:      opts.notBefore,
		NotAfter:       opts.notAfter,
		KeyUsage:       x509.KeyUsageDigitalSignature,
		ExtKeyUsage:    opts.extKeyUsage,
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

// spiffeID returns the SPIFFE ID csi-driver-spiffe issues to component in the test namespace.
func spiffeID(component string) string {
	return SPIFFEID(DefaultTrustDomain, testNamespace, component)
}

// spiffeURIs returns the URI SANs csi-driver-spiffe issues to component.
func spiffeURIs(component string) []string {
	return []string{spiffeID(component)}
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
	writeFiles(t, dir, ca.issue(t, certOptions{uris: spiffeURIs(component)}), ca.pem)
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

func TestSPIFFEID(t *testing.T) {
	t.Parallel()
	require.Equal(t, "spiffe://radius.internal/ns/radius-system/sa/applications-rp", SPIFFEID("radius.internal", "radius-system", "applications-rp"))
	require.Equal(t, "spiffe://example.org/ns/custom/sa/ucp", SPIFFEID("example.org", "custom", "ucp"))
}

func TestCheckSPIFFEID(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	tests := []struct {
		name string
		opts certOptions
		want string
	}{
		{name: "matching", opts: certOptions{uris: spiffeURIs("ucp")}},
		{name: "mixed DNS SAN", opts: certOptions{uris: spiffeURIs("ucp"), dnsNames: []string{"attacker.example.com"}}, want: "non-URI SAN"},
		{name: "mixed IP SAN", opts: certOptions{uris: spiffeURIs("ucp"), ipAddresses: []net.IP{net.ParseIP("127.0.0.1")}}, want: "non-URI SAN"},
		{name: "mixed email SAN", opts: certOptions{uris: spiffeURIs("ucp"), emails: []string{"service@example.test"}}, want: "non-URI SAN"},
		{name: "no URI SAN", opts: certOptions{dnsNames: []string{"ucp.radius-system.svc"}}, want: "no URI SAN"},
		{name: "other component", opts: certOptions{uris: spiffeURIs("dynamic-rp")}, want: "is not"},
		{name: "other namespace", opts: certOptions{uris: []string{SPIFFEID(DefaultTrustDomain, "other", "ucp")}}, want: "is not"},
		{name: "other trust domain", opts: certOptions{uris: []string{SPIFFEID("example.org", testNamespace, "ucp")}}, want: "is not"},
		{name: "two URI SANs", opts: certOptions{uris: []string{spiffeID("ucp"), spiffeID("dynamic-rp")}}, want: "more than one URI SAN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := checkSPIFFEID(ca.issue(t, tt.opts).cert, spiffeID("ucp"))
			if tt.want == "" {
				require.NoError(t, err)
				return
			}
			requireCode(t, err, authz.CodePeerCertificateInvalid)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestCheckKey(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	tests := []struct {
		name string
		key  crypto.Signer
		want string
	}{
		{name: "P-256", key: ecKey(t, elliptic.P256())},
		{name: "P-384", key: ecKey(t, elliptic.P384())},
		{name: "RSA-2048", key: rsaKey(t, 2048)},
		{name: "P-224", key: ecKey(t, elliptic.P224()), want: "ECDSA curve P-224"},
		{name: "RSA-1024", key: rsaKey(t, 1024), want: "RSA key size 1024"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := checkKey(ca.issue(t, certOptions{uris: spiffeURIs("ucp"), key: tt.key}).cert)
			if tt.want == "" {
				require.NoError(t, err)
				return
			}
			requireCode(t, err, authz.CodePeerCertificateInvalid)
			require.ErrorContains(t, err, tt.want)
		})
	}

	t.Run("Ed25519", func(t *testing.T) {
		t.Parallel()
		_, key, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		err = checkKey(ca.issue(t, certOptions{uris: spiffeURIs("ucp"), key: key}).cert)
		requireCode(t, err, authz.CodePeerCertificateInvalid)
		require.ErrorContains(t, err, "ed25519")
	})
}

func TestError(t *testing.T) {
	t.Parallel()
	inner := peerError("inner", nil)
	err := peerError("outer", inner)
	require.Equal(t, "PeerCertificateInvalid: outer: PeerCertificateInvalid: inner", err.Error())
	require.ErrorIs(t, err, inner)
}
