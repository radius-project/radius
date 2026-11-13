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
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	// CertFile, KeyFile, and CAFile are the file names that csi-driver-spiffe writes, which
	// match a mounted kubernetes.io/tls Secret.
	CertFile = "tls.crt"
	KeyFile  = "tls.key"
	CAFile   = "ca.crt"

	// DefaultPollInterval is how often Start checks the files for changes.
	DefaultPollInterval = 10 * time.Second

	// maxReadAttempts bounds how many times Reload reads the files while they keep changing.
	maxReadAttempts = 5
)

// material is one consistent set of certificate, key, and CA bundle. cert is nil for a
// trust-only Source.
type material struct {
	cert *tls.Certificate
	pool *x509.CertPool
}

// Source holds a component's certificate, key, and CA bundle loaded from a directory,
// such as a csi-driver-spiffe volume. Start reloads them when the files change. Kubelet
// and CSI drivers update volumes by swapping a symlink, so Source polls file contents
// rather than watching individual files.
type Source struct {
	dir       string
	caDir     string
	trustOnly bool
	now       func() time.Time
	readFile  func(string) ([]byte, error)

	mu      sync.Mutex
	digest  [sha256.Size]byte
	current atomic.Pointer[material]
}

// NewSource loads the certificate, key, and CA bundle from dir. It returns an error if
// any file is missing or invalid, or if the certificate has expired. Servers use it.
func NewSource(dir string) (*Source, error) {
	return newSource(dir, dir, false)
}

// NewSourceWithCA loads the certificate and key from dir and the CA bundle (ca.crt) from
// caDir. Radius receivers use it because csi-driver-spiffe writes only tls.crt and tls.key;
// the Radius CA bundle is mounted separately from the radius-ca Secret.
func NewSourceWithCA(dir, caDir string) (*Source, error) {
	return newSource(dir, caDir, false)
}

// NewTrustSource loads only the CA bundle (ca.crt) from dir. Clients use it to verify
// servers; they do not present a certificate.
func NewTrustSource(dir string) (*Source, error) {
	return newSource(dir, dir, true)
}

func newSource(dir, caDir string, trustOnly bool) (*Source, error) {
	s := &Source{dir: dir, caDir: caDir, trustOnly: trustOnly, now: time.Now, readFile: os.ReadFile}
	if _, err := s.Reload(); err != nil {
		return nil, err
	}
	return s, nil
}

// files returns the paths of the files the Source reads, in the order parse expects.
func (s *Source) files() []string {
	ca := filepath.Join(s.caDir, CAFile)
	if s.trustOnly {
		return []string{ca}
	}
	return []string{filepath.Join(s.dir, CertFile), filepath.Join(s.dir, KeyFile), ca}
}

// Reload reads the files and, if they changed and are valid, replaces the current
// material. It reports whether the material changed. On error the previous material
// stays in use.
func (s *Source) Reload() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, digest, err := s.readStable()
	if err != nil {
		return false, err
	}
	if s.current.Load() != nil && digest == s.digest {
		return false, nil
	}

	m, err := s.parse(raw)
	if err != nil {
		return false, err
	}

	s.digest = digest
	s.current.Store(m)
	return true, nil
}

// readStable reads the files until two consecutive reads match, so a Secret update
// between individual file reads cannot mix files from two versions.
func (s *Source) readStable() ([][]byte, [sha256.Size]byte, error) {
	var prev [sha256.Size]byte
	paths := s.files()
	for attempt := range maxReadAttempts {
		raw := make([][]byte, len(paths))
		for i, path := range paths {
			data, err := s.readFile(path)
			if err != nil {
				return nil, prev, fmt.Errorf("failed to read %s: %w", filepath.Base(path), err)
			}
			raw[i] = data
		}

		digest := sha256.Sum256(bytes.Join(raw, []byte{0}))
		if attempt > 0 && digest == prev {
			return raw, digest, nil
		}
		prev = digest
	}
	return nil, prev, errors.New("certificate files kept changing while being read")
}

// parse builds material from the file contents read in the order of files.
func (s *Source) parse(raw [][]byte) (*material, error) {
	caPEM := raw[len(raw)-1]
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("failed to load CA bundle: no certificates found")
	}
	if s.trustOnly {
		return &material{pool: pool}, nil
	}

	cert, err := tls.X509KeyPair(raw[0], raw[1])
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate and key: %w", err)
	}
	if err := s.checkExpiry(&cert); err != nil {
		return nil, err
	}

	// The certificate must chain to the CA bundle it is loaded with, so peers that trust
	// this bundle also accept this certificate.
	intermediates := x509.NewCertPool()
	for _, der := range cert.Certificate[1:] {
		c, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, fmt.Errorf("failed to parse intermediate certificate: %w", err)
		}
		intermediates.AddCert(c)
	}
	if _, err := cert.Leaf.Verify(x509.VerifyOptions{
		Roots:         pool,
		Intermediates: intermediates,
		CurrentTime:   s.now(),
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}); err != nil {
		return nil, fmt.Errorf("certificate is not issued by the CA bundle: %w", err)
	}

	return &material{cert: &cert, pool: pool}, nil
}

func (s *Source) checkExpiry(cert *tls.Certificate) error {
	if !s.isValid(cert.Leaf) {
		return &Error{Code: authz.CodeCertificateExpired, Reason: "own certificate is outside its validity period"}
	}
	return nil
}

// isValid reports whether leaf is within its validity period.
func (s *Source) isValid(leaf *x509.Certificate) bool {
	now := s.now()
	return !now.After(leaf.NotAfter) && !now.Before(leaf.NotBefore)
}

// Start polls the files every interval until ctx is done, reloading them when they
// change. Errors are logged and the previous material stays in use.
func (s *Source) Start(ctx context.Context, interval time.Duration) {
	logger := ucplog.FromContextOrDiscard(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			changed, err := s.Reload()
			if err != nil {
				logger.Error(err, "Failed to reload TLS certificate", "dir", s.dir)
			} else if changed {
				logger.Info("Reloaded TLS certificate", "dir", s.dir)
			}
		}
	}
}

// load returns the current material, so callers use a certificate and CA bundle from
// the same version. It fails closed if the certificate has expired, for example after
// a failed rotation.
func (s *Source) load() (*material, error) {
	m := s.current.Load()
	if m.cert == nil {
		return m, nil
	}
	if err := s.checkExpiry(m.cert); err != nil {
		return nil, err
	}
	return m, nil
}
