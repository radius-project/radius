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
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// StartupProofPath is the public-only initialization endpoint shared with the deployment engine.
const StartupProofPath = "/healthz/ucp-mtls"

const maxProofCAs = 16

var processBootID = uuid.NewString()

// StartupEvidence describes current local TLS initialization, not a remote handshake.
type StartupEvidence struct {
	SchemaVersion         int       `json:"schemaVersion"`
	Ready                 bool      `json:"ready"`
	ProofKind             string    `json:"proofKind"`
	PodUID                string    `json:"podUid"`
	BootID                string    `json:"bootId"`
	InitializedAt         time.Time `json:"initializedAt"`
	TrustBundleSHA256     string    `json:"trustBundleSha256"`
	CASHA256              []string  `json:"caSha256"`
	LeafSHA256            string    `json:"leafSha256"`
	NotBefore             time.Time `json:"notBefore"`
	NotAfter              time.Time `json:"notAfter"`
	PeerHandshakeVerified bool      `json:"peerHandshakeVerified"`
}

func fingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}

func validFingerprint(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && strings.ToLower(value) == value
}

func validProofUUID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}

func digestFingerprints(fingerprints []string) string {
	return fingerprint([]byte(strings.Join(fingerprints, "\n")))
}

type publicTrust struct {
	fingerprints []string
	digest       string
	notBefore    time.Time
	notAfter     time.Time
}

func boundPublicTrust(m *material, trust publicTrust) {
	if trust.notBefore.After(m.notBefore) {
		m.notBefore = trust.notBefore
	}
	if trust.notAfter.Before(m.notAfter) {
		m.notAfter = trust.notAfter
	}
}

func parsePublicTrust(data []byte, now time.Time) (publicTrust, error) {
	var trust publicTrust
	seen := map[string]bool{}
	for rest := bytes.TrimSpace(data); len(rest) > 0; {
		if !bytes.HasPrefix(rest, []byte("-----BEGIN CERTIFICATE-----")) {
			return trust, errors.New("trust must contain only CA certificates")
		}
		block, remaining := pem.Decode(rest)
		if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 ||
			bytes.Count(rest[:len(rest)-len(remaining)], []byte("-----BEGIN ")) != 1 {
			return trust, errors.New("invalid CA PEM")
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return trust, errors.New("invalid CA certificate")
		}
		if !cert.IsCA || !cert.BasicConstraintsValid || cert.KeyUsage&x509.KeyUsageCertSign == 0 ||
			len(cert.UnhandledCriticalExtensions) > 0 || checkKey(cert) != nil {
			return trust, errors.New("unsupported signing CA")
		}
		if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
			return trust, errors.New("CA is outside its validity period")
		}
		hash := fingerprint(cert.Raw)
		if !seen[hash] {
			seen[hash] = true
			trust.fingerprints = append(trust.fingerprints, hash)
			if len(seen) > maxProofCAs {
				return trust, errors.New("too many signing CAs")
			}
			if cert.NotBefore.After(trust.notBefore) {
				trust.notBefore = cert.NotBefore
			}
			if trust.notAfter.IsZero() || cert.NotAfter.Before(trust.notAfter) {
				trust.notAfter = cert.NotAfter
			}
		}
		rest = bytes.TrimSpace(remaining)
	}
	if len(seen) == 0 {
		return trust, errors.New("CA bundle is empty")
	}
	slices.Sort(trust.fingerprints)
	trust.digest = digestFingerprints(trust.fingerprints)
	return trust, nil
}

// TrustBundleDigest returns the agreed Go/.NET digest and sorted, unique CA DER fingerprints.
// It accepts only currently valid dedicated signing CA certificates, never private material.
func TrustBundleDigest(data []byte, now time.Time) (string, []string, error) {
	trust, err := parsePublicTrust(data, now)
	return trust.digest, trust.fingerprints, err
}

// Validate checks a report against independently observed pod identity and expected public material.
func (e StartupEvidence) Validate(podUID, trustDigest, leafDigest string, now time.Time) error {
	if e.SchemaVersion != 1 || !e.Ready || e.ProofKind != "startup-tls-initialized" || e.PeerHandshakeVerified {
		return errors.New("unsupported or unready TLS initialization evidence")
	}
	if !validProofUUID(e.BootID) || !validProofUUID(podUID) {
		return errors.New("invalid TLS process identity")
	}
	if e.PodUID != podUID || e.InitializedAt.IsZero() || e.InitializedAt.After(now) ||
		e.NotBefore.IsZero() || e.NotAfter.IsZero() || e.NotAfter.Before(e.NotBefore) ||
		now.Before(e.NotBefore) || now.After(e.NotAfter) {
		return errors.New("TLS initialization evidence has stale identity or validity")
	}
	if !validFingerprint(trustDigest) || !validFingerprint(leafDigest) || e.TrustBundleSHA256 != trustDigest ||
		e.LeafSHA256 != leafDigest || len(e.CASHA256) == 0 || len(e.CASHA256) > maxProofCAs {
		return errors.New("TLS initialization evidence does not match expected material")
	}
	for i, hash := range e.CASHA256 {
		if !validFingerprint(hash) || (i > 0 && e.CASHA256[i-1] >= hash) {
			return errors.New("TLS initialization evidence has invalid CA fingerprints")
		}
	}
	if digestFingerprints(e.CASHA256) != trustDigest {
		return errors.New("TLS initialization evidence has inconsistent trust digest")
	}
	return nil
}

// StartupProof pins trust on an actual TLS Source. Every listener or transport covered
// by its evidence must use that Source. It does not report hosted-service health.
type StartupProof struct {
	source        *Source
	podUID        string
	bootID        string
	initializedAt time.Time
}

// NewStartupProof validates and pins the expected trust on source without pinning its leaf.
// Valid daily leaf renewals remain supported; a different trust set requires a new process.
func NewStartupProof(source *Source, podUID, expectedTrust string) (*StartupProof, error) {
	if source == nil {
		return nil, errors.New("TLS source is required")
	}
	if !validProofUUID(podUID) || !validFingerprint(expectedTrust) {
		return nil, errors.New("invalid TLS startup proof configuration")
	}
	if _, err := source.Reload(); err != nil {
		return nil, errors.New("cannot initialize TLS startup proof")
	}
	source.mu.Lock()
	defer source.mu.Unlock()
	m, err := source.loadCurrent()
	if err != nil {
		return nil, errors.New("cannot initialize TLS startup proof")
	}
	trust, err := parsePublicTrust(m.caPEM, source.now())
	if err != nil || trust.digest != expectedTrust ||
		(source.expectedTrust != "" && source.expectedTrust != expectedTrust) {
		return nil, errors.New("TLS startup proof trust does not match")
	}
	if source.startupProof != nil {
		if source.startupProof.podUID != podUID {
			return nil, errors.New("TLS startup proof pod identity cannot change within a process")
		}
		return source.startupProof, nil
	}
	source.expectedTrust = expectedTrust
	pinned := *m
	boundPublicTrust(&pinned, trust)
	source.current.Store(&pinned)
	source.proofMode.Store(true)
	source.startupProof = &StartupProof{source: source, podUID: podUID, bootID: processBootID, initializedAt: source.now().UTC()}
	return source.startupProof, nil
}

// Snapshot refreshes the same source used by TLS before reporting its current material.
func (p *StartupProof) Snapshot() (StartupEvidence, error) {
	if _, err := p.source.Reload(); err != nil {
		return StartupEvidence{}, errors.New("TLS startup proof refresh failed")
	}
	p.source.mu.Lock()
	defer p.source.mu.Unlock()
	m, err := p.source.loadCurrent()
	if err != nil {
		return StartupEvidence{}, errors.New("TLS startup proof is outside its validity period")
	}
	trust, err := parsePublicTrust(m.caPEM, p.source.now())
	if err != nil || trust.digest != p.source.expectedTrust {
		return StartupEvidence{}, errors.New("TLS startup proof trust does not match")
	}
	notBefore, notAfter := m.notBefore, m.notAfter
	if trust.notBefore.After(notBefore) {
		notBefore = trust.notBefore
	}
	if trust.notAfter.Before(notAfter) {
		notAfter = trust.notAfter
	}
	return StartupEvidence{SchemaVersion: 1, Ready: true, ProofKind: "startup-tls-initialized",
		PodUID: p.podUID, BootID: p.bootID, InitializedAt: p.initializedAt,
		TrustBundleSHA256: trust.digest, CASHA256: trust.fingerprints, LeafSHA256: fingerprint(m.cert.Leaf.Raw),
		NotBefore: notBefore.UTC(), NotAfter: notAfter.UTC()}, nil
}

// ServeHTTP returns bounded public status. It grants no component identity or proxy access.
func (p *StartupProof) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	evidence, err := p.Snapshot()
	if err != nil {
		ucplog.FromContextOrDiscard(r.Context()).Error(err, "TLS startup proof unavailable")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("{\"schemaVersion\":1,\"ready\":false}\n"))
		return
	}
	if err := json.NewEncoder(w).Encode(evidence); err != nil {
		ucplog.FromContextOrDiscard(r.Context()).Error(err, "Failed to write TLS startup proof")
	}
}
