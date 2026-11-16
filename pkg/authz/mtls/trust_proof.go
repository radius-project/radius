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
	"errors"
	"time"
)

// TrustEvidence reports only the actual outbound authority's loaded anchors.
// It proves neither token readiness nor a peer handshake or serving certificate.
type TrustEvidence struct {
	SchemaVersion          int       `json:"schemaVersion"`
	Ready                  bool      `json:"ready"`
	ProofKind              string    `json:"proofKind"`
	PodUID                 string    `json:"podUid"`
	BootID                 string    `json:"bootId"`
	InitializedAt          time.Time `json:"initializedAt"`
	TrustBundleSHA256      string    `json:"trustBundleSha256"`
	CASHA256               []string  `json:"caSha256"`
	NotBefore              time.Time `json:"notBefore"`
	NotAfter               time.Time `json:"notAfter"`
	ExpectedServerSPIFFEID string    `json:"expectedServerSpiffeId"`
	PeerHandshakeVerified  bool      `json:"peerHandshakeVerified"`
}

// Validate checks schema2 against independently pinned pod, trust and UCP identity.
func (e TrustEvidence) Validate(podUID, trustDigest, serverID string, now time.Time) error {
	if e.SchemaVersion != 2 || !e.Ready || e.ProofKind != "ucp-server-trust-initialized" ||
		e.PeerHandshakeVerified || e.ExpectedServerSPIFFEID != serverID || serverID == "" {
		return errors.New("unsupported or unready outbound trust evidence")
	}
	common := StartupEvidence{
		PodUID: e.PodUID, BootID: e.BootID, InitializedAt: e.InitializedAt,
		TrustBundleSHA256: e.TrustBundleSHA256, CASHA256: e.CASHA256,
		NotBefore: e.NotBefore, NotAfter: e.NotAfter}
	return validateEvidenceMetadata(common, podUID, trustDigest, now)
}

// TrustProof binds schema2 evidence to the same Source used by outbound transports.
// Hosts must authenticate a caller before exposing Snapshot; it registers no route.
type TrustProof struct {
	source        *Source
	podUID        string
	serverID      string
	bootID        string
	initializedAt time.Time
}

// NewTrustProof pins only a trust-only authority, never a CSI leaf source.
func NewTrustProof(source *Source, podUID, expectedTrust, serverID string) (*TrustProof, error) {
	if source == nil || !source.trustOnly || !validProofUUID(podUID) ||
		!validFingerprint(expectedTrust) || serverID == "" {
		return nil, errors.New("invalid trust-only proof configuration")
	}
	if _, err := source.Reload(); err != nil {
		return nil, err
	}
	source.mu.Lock()
	defer source.mu.Unlock()
	m, err := source.loadCurrent()
	if err != nil {
		return nil, err
	}
	trust, err := parsePublicTrust(m.caPEM, source.now())
	if err != nil || trust.digest != expectedTrust ||
		source.expectedTrust != "" && source.expectedTrust != expectedTrust {
		return nil, errors.New("outbound authority differs from pinned trust")
	}
	if source.trustProof != nil {
		if source.trustProof.podUID != podUID || source.trustProof.serverID != serverID {
			return nil, errors.New("outbound proof identity cannot change within a process")
		}
		return source.trustProof, nil
	}
	source.expectedTrust = expectedTrust
	source.trustProof = &TrustProof{source: source, podUID: podUID, serverID: serverID,
		bootID: processBootID, initializedAt: source.now().UTC()}
	return source.trustProof, nil
}

// Snapshot refreshes and reads the actual authority under its generation lock.
func (p *TrustProof) Snapshot() (TrustEvidence, error) {
	if _, err := p.source.Reload(); err != nil {
		return TrustEvidence{}, err
	}
	p.source.mu.Lock()
	defer p.source.mu.Unlock()
	m, err := p.source.loadCurrent()
	if err != nil {
		return TrustEvidence{}, err
	}
	trust, err := parsePublicTrust(m.caPEM, p.source.now())
	if err != nil || trust.digest != p.source.expectedTrust {
		return TrustEvidence{}, errors.New("outbound authority differs from pinned trust")
	}
	return TrustEvidence{SchemaVersion: 2, Ready: true, ProofKind: "ucp-server-trust-initialized",
		PodUID: p.podUID, BootID: p.bootID, InitializedAt: p.initializedAt,
		TrustBundleSHA256: trust.digest, CASHA256: trust.fingerprints,
		NotBefore: trust.notBefore.UTC(), NotAfter: trust.notAfter.UTC(),
		ExpectedServerSPIFFEID: p.serverID}, nil
}
