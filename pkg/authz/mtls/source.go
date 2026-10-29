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
	// CertFile, KeyFile, and CAFile are the file names in a mounted kubernetes.io/tls Secret.
	CertFile = "tls.crt"
	KeyFile  = "tls.key"
	CAFile   = "ca.crt"

	// DefaultPollInterval is how often Start checks the files for changes.
	DefaultPollInterval = 10 * time.Second

	// maxReadAttempts bounds how many times Reload reads the files while they keep changing.
	maxReadAttempts = 5
)

// material is one consistent set of certificate, key, and CA bundle.
type material struct {
	cert *tls.Certificate
	pool *x509.CertPool
}

// Source holds a component's certificate, key, and CA bundle loaded from a directory,
// such as a mounted Secret. Start reloads them when the files change. Kubelet updates
// Secret volumes by swapping a symlink, so Source polls file contents rather than
// watching individual files.
type Source struct {
	dir      string
	now      func() time.Time
	readFile func(string) ([]byte, error)

	mu      sync.Mutex
	digest  [sha256.Size]byte
	current atomic.Pointer[material]
}

// NewSource loads the certificate, key, and CA bundle from dir. It returns an error if
// any file is missing or invalid, or if the certificate has expired.
func NewSource(dir string) (*Source, error) {
	s := &Source{dir: dir, now: time.Now, readFile: os.ReadFile}
	if _, err := s.Reload(); err != nil {
		return nil, err
	}
	return s, nil
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

	m, err := s.parse(raw[0], raw[1], raw[2])
	if err != nil {
		return false, err
	}

	s.digest = digest
	s.current.Store(m)
	return true, nil
}

// readStable reads the files until two consecutive reads match, so a Secret update
// between individual file reads cannot mix files from two versions.
func (s *Source) readStable() ([3][]byte, [sha256.Size]byte, error) {
	var prev [sha256.Size]byte
	for attempt := range maxReadAttempts {
		var raw [3][]byte
		for i, name := range []string{CertFile, KeyFile, CAFile} {
			data, err := s.readFile(filepath.Join(s.dir, name))
			if err != nil {
				return raw, prev, fmt.Errorf("failed to read %s: %w", name, err)
			}
			raw[i] = data
		}

		digest := sha256.Sum256(bytes.Join(raw[:], []byte{0}))
		if attempt > 0 && digest == prev {
			return raw, digest, nil
		}
		prev = digest
	}
	return [3][]byte{}, prev, errors.New("certificate files kept changing while being read")
}

func (s *Source) parse(certPEM, keyPEM, caPEM []byte) (*material, error) {
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate and key: %w", err)
	}
	if err := s.checkExpiry(&cert); err != nil {
		return nil, err
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("failed to load CA bundle: no certificates found")
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
	now := s.now()
	if now.After(cert.Leaf.NotAfter) || now.Before(cert.Leaf.NotBefore) {
		return &Error{Code: authz.CodeCertificateExpired, Reason: "own certificate is outside its validity period"}
	}
	return nil
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
				logger.Error(err, "Failed to reload mTLS certificate", "dir", s.dir)
			} else if changed {
				logger.Info("Reloaded mTLS certificate", "dir", s.dir)
			}
		}
	}
}

// load returns the current material, so callers use a certificate and CA bundle from
// the same version. It fails closed if the certificate has expired, for example after
// a failed rotation.
func (s *Source) load() (*material, error) {
	m := s.current.Load()
	if err := s.checkExpiry(m.cert); err != nil {
		return nil, err
	}
	return m, nil
}
