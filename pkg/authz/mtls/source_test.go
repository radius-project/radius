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
	"context"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// kubeletDir mimics a Secret volume: files are symlinks into ..data, which points at
// a timestamped directory that kubelet swaps atomically on update.
type kubeletDir struct {
	root string
	n    int
}

// currentCert returns the certificate of the source's current material.
func currentCert(src *Source) (*tls.Certificate, error) {
	m, err := src.load()
	if err != nil {
		return nil, err
	}
	return m.cert, nil
}

func newKubeletDir(t *testing.T, cert testCert, caPEM []byte) *kubeletDir {
	t.Helper()
	k := &kubeletDir{root: t.TempDir()}
	k.update(t, cert, caPEM)
	for _, name := range []string{CertFile, KeyFile, CAFile} {
		require.NoError(t, os.Symlink(filepath.Join("..data", name), filepath.Join(k.root, name)))
	}
	return k
}

func (k *kubeletDir) update(t *testing.T, cert testCert, caPEM []byte) {
	t.Helper()
	require.NoError(t, k.stage(t, cert, caPEM)())
}

// stage writes a new version and returns a function that swaps ..data to it, as kubelet does.
func (k *kubeletDir) stage(t *testing.T, cert testCert, caPEM []byte) func() error {
	t.Helper()
	k.n++
	version := filepath.Join(k.root, fmt.Sprintf("..version%d", k.n))
	require.NoError(t, os.Mkdir(version, 0o700))
	writeFiles(t, version, cert, caPEM)

	tmp := filepath.Join(k.root, "..data_tmp")
	require.NoError(t, os.Symlink(filepath.Base(version), tmp))
	return func() error { return os.Rename(tmp, filepath.Join(k.root, "..data")) }
}

func TestNewSource_Errors(t *testing.T) {
	t.Parallel()
	ca := newCA(t)

	t.Run("missing files", func(t *testing.T) {
		t.Parallel()
		_, err := NewSource(t.TempDir())
		require.Error(t, err)
	})

	t.Run("expired own certificate", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFiles(t, dir, ca.issue(t, certOptions{
			dnsNames:  componentSANs("ucp"),
			notBefore: time.Now().Add(-2 * time.Hour),
			notAfter:  time.Now().Add(-time.Hour),
		}), ca.pem)
		_, err := NewSource(dir)
		requireCode(t, err, "CertificateExpired")
	})

	t.Run("empty CA bundle", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFiles(t, dir, ca.issue(t, certOptions{dnsNames: componentSANs("ucp")}), []byte("not a certificate"))
		_, err := NewSource(dir)
		require.Error(t, err)
	})

	t.Run("certificate from another CA", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFiles(t, dir, newCA(t).issue(t, certOptions{dnsNames: componentSANs("ucp")}), ca.pem)
		_, err := NewSource(dir)
		require.ErrorContains(t, err, "not issued by the CA bundle")
	})
}

func TestSource_DedicatedIntermediateCA(t *testing.T) {
	t.Parallel()
	root := newCA(t)
	newIntermediate := func(t *testing.T, parent testCA, name string) testCA {
		t.Helper()
		key := ecKey(t, elliptic.P256())
		template := &x509.Certificate{
			SerialNumber:                big.NewInt(2),
			Subject:                     pkix.Name{CommonName: name},
			NotBefore:                   parent.cert.NotBefore,
			NotAfter:                    parent.cert.NotAfter,
			IsCA:                        true,
			BasicConstraintsValid:       true,
			MaxPathLenZero:              true,
			KeyUsage:                    x509.KeyUsageCertSign,
			PermittedDNSDomainsCritical: true,
			PermittedDNSDomains:         []string{testNamespace + ".svc"},
		}
		der, err := x509.CreateCertificate(rand.Reader, template, parent.cert, key.Public(), parent.key)
		require.NoError(t, err)
		cert, err := x509.ParseCertificate(der)
		require.NoError(t, err)
		return testCA{cert: cert, key: key, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
	}
	dedicated := newIntermediate(t, root, "radius-dedicated-ca")
	sibling := newIntermediate(t, root, "other-team-ca")

	newSource := func(t *testing.T, ca testCA, component string, trust []byte) *Source {
		t.Helper()
		cert := ca.issue(t, certOptions{dnsNames: componentSANs(component)})
		// cert-manager may return the organization chain in tls.crt. Only the
		// separately projected dedicated CA is a trust anchor, not that chain.
		cert.certPEM = append(cert.certPEM, ca.pem...)
		cert.certPEM = append(cert.certPEM, root.pem...)
		dir := t.TempDir()
		writeFiles(t, dir, cert, trust)
		src, err := NewSource(dir)
		require.NoError(t, err)
		return src
	}
	server := newSource(t, dedicated, "ucp", dedicated.pem)
	client := newSource(t, dedicated, "controller", dedicated.pem)
	result := handshake(t, server.ServerConfig(testNamespace), clientConfig(t, client, ucpServerName))
	require.NoError(t, result.serverErr)
	require.NoError(t, result.clientErr)
	require.Len(t, result.clientState.VerifiedChains, 1)
	require.Equal(t, dedicated.cert.Raw, result.clientState.VerifiedChains[0][1].Raw)
	require.Len(t, result.clientState.VerifiedChains[0], 2)

	for _, tt := range []struct {
		name string
		ca   testCA
	}{
		{name: "organization root", ca: root},
		{name: "sibling intermediate", ca: sibling},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Even a canonical Radius SAN is insufficient under another CA
			// from the same organization.
			otherClient := newSource(t, tt.ca, "controller", root.pem)
			result := handshake(t, server.ServerConfig(testNamespace), clientConfig(t, otherClient, ucpServerName))
			require.ErrorContains(t, result.serverErr, "unknown authority")

			otherServer := newSource(t, tt.ca, "ucp", root.pem)
			result = handshake(t, otherServer.ServerConfig(testNamespace), clientConfig(t, client, ucpServerName))
			require.ErrorContains(t, result.clientErr, "unknown authority")
		})
	}

	t.Run("name constraint rejects another namespace", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		cert := dedicated.issue(t, certOptions{dnsNames: []string{"ucp.other-system.svc"}})
		writeFiles(t, dir, cert, dedicated.pem)
		_, err := NewSource(dir)
		require.ErrorContains(t, err, "not issued by the CA bundle")
	})

	t.Run("path length zero rejects a subordinate CA", func(t *testing.T) {
		t.Parallel()
		subordinate := newIntermediate(t, dedicated, "subordinate-ca")
		cert := subordinate.issue(t, certOptions{dnsNames: componentSANs("ucp")})
		cert.certPEM = append(cert.certPEM, subordinate.pem...)
		dir := t.TempDir()
		writeFiles(t, dir, cert, dedicated.pem)
		_, err := NewSource(dir)
		require.ErrorContains(t, err, "too many intermediates")
	})
}

func TestSource_CertificateExpiresAfterLoad(t *testing.T) {
	t.Parallel()
	src := newTestSource(t, newCA(t), "ucp")
	src.now = func() time.Time { return time.Now().Add(2 * time.Hour) }

	_, err := src.ClientConfig(testNamespace)
	requireCode(t, err, "CertificateExpired")
}

func TestSource_Reload(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	first := ca.issue(t, certOptions{dnsNames: componentSANs("ucp")})
	dir := newKubeletDir(t, first, ca.pem)

	src, err := NewSource(dir.root)
	require.NoError(t, err)

	changed, err := src.Reload()
	require.NoError(t, err)
	require.False(t, changed)

	// Invalid files keep the previous certificate.
	require.NoError(t, os.WriteFile(filepath.Join(dir.root, "..data", KeyFile), []byte("bad"), 0o600))
	_, err = src.Reload()
	require.Error(t, err)
	cert, err := currentCert(src)
	require.NoError(t, err)
	require.Equal(t, first.cert.Raw, cert.Leaf.Raw)

	// A rotated certificate and CA are picked up by Start.
	rotatedCA := newCA(t)
	second := rotatedCA.issue(t, certOptions{dnsNames: componentSANs("ucp")})
	dir.update(t, second, rotatedCA.pem)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	go src.Start(ctx, 10*time.Millisecond)

	require.Eventually(t, func() bool {
		cert, err := currentCert(src)
		return err == nil && string(cert.Leaf.Raw) == string(second.cert.Raw)
	}, 5*time.Second, 10*time.Millisecond)

	// Handshakes use the reloaded certificate and CA without a new config.
	client := newTestSource(t, rotatedCA, "controller")
	result := handshake(t, src.ServerConfig(testNamespace), clientConfig(t, client, ucpServerName))
	require.NoError(t, result.serverErr)
	require.NoError(t, result.clientErr)
	require.Equal(t, second.cert.Raw, result.clientState.PeerCertificates[0].Raw)
}

func TestSource_ReloadDuringUpdate(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	dir := newKubeletDir(t, ca.issue(t, certOptions{dnsNames: componentSANs("ucp")}), ca.pem)
	src, err := NewSource(dir.root)
	require.NoError(t, err)

	// Swap the Secret after reading the key. The attempt must stay bound to the
	// old target, detect the swap and retry the complete new generation.
	rotatedCA := newCA(t)
	second := rotatedCA.issue(t, certOptions{dnsNames: componentSANs("ucp")})
	swapped := false
	src.readFile = func(name string) ([]byte, error) {
		data, err := os.ReadFile(name)
		if !swapped && filepath.Base(name) == KeyFile {
			swapped = true
			dir.update(t, second, rotatedCA.pem)
		}
		return data, err
	}

	changed, err := src.Reload()
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, swapped)

	m, err := src.load()
	require.NoError(t, err)
	require.Equal(t, second.cert.Raw, m.cert.Leaf.Raw)
	_, err = m.cert.Leaf.Verify(x509.VerifyOptions{Roots: m.pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}})
	require.NoError(t, err)
}

func TestSource_ReloadKeepsChanging(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	dir := newKubeletDir(t, ca.issue(t, certOptions{dnsNames: componentSANs("ucp")}), ca.pem)
	src, err := NewSource(dir.root)
	require.NoError(t, err)
	before, err := currentCert(src)
	require.NoError(t, err)

	reads := 0
	src.readFile = func(name string) ([]byte, error) {
		reads++
		data, err := os.ReadFile(name)
		return append(data, byte(reads)), err
	}

	_, err = src.Reload()
	require.ErrorContains(t, err, "kept changing")
	after, err := currentCert(src)
	require.NoError(t, err)
	require.Same(t, before, after)
}
