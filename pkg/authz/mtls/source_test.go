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
	"crypto/x509"
	"fmt"
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
	k.n++
	version := filepath.Join(k.root, fmt.Sprintf("..version%d", k.n))
	require.NoError(t, os.Mkdir(version, 0o700))
	writeFiles(t, version, cert, caPEM)

	tmp := filepath.Join(k.root, "..data_tmp")
	require.NoError(t, os.Symlink(filepath.Base(version), tmp))
	require.NoError(t, os.Rename(tmp, filepath.Join(k.root, "..data")))
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

func TestSource_CertificateExpiresAfterLoad(t *testing.T) {
	t.Parallel()
	src := newTestSource(t, newCA(t), "ucp")
	src.now = func() time.Time { return time.Now().Add(2 * time.Hour) }

	_, err := src.ClientConfig(testNamespace).GetClientCertificate(nil)
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
	cert, err := src.certificate()
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
		cert, err := src.certificate()
		return err == nil && string(cert.Leaf.Raw) == string(second.cert.Raw)
	}, 5*time.Second, 10*time.Millisecond)

	// Handshakes use the reloaded certificate and CA without a new config.
	client := newTestSource(t, rotatedCA, "controller")
	result := handshake(t, src.ServerConfig(testNamespace), clientConfig(client, ucpServerName))
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

	// Swap the Secret after the first read of the key, so that read sees the old
	// certificate and key with the new CA bundle.
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
	before, err := src.certificate()
	require.NoError(t, err)

	reads := 0
	src.readFile = func(name string) ([]byte, error) {
		reads++
		data, err := os.ReadFile(name)
		return append(data, byte(reads)), err
	}

	_, err = src.Reload()
	require.ErrorContains(t, err, "kept changing")
	after, err := src.certificate()
	require.NoError(t, err)
	require.Same(t, before, after)
}
