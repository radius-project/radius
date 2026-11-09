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
	"crypto/x509"
	"errors"
	"net/http"
	"time"
)

// AggregationProofPath reports the aggregation listener's actual serving Source,
// independently of component mTLS or the external provider's token identities.
const AggregationProofPath = "/healthz/ucp-aggregation-tls"

const aggregationProofKind = "aggregation-serving-tls-initialized"

// AggregationEvidence cannot substitute for component transport evidence.
type AggregationEvidence StartupEvidence

// Validate binds serving evidence to the expected pod, dedicated CA and leaf.
func (e AggregationEvidence) Validate(podUID, trustDigest, leafDigest string, now time.Time) error {
	if e.ProofKind != aggregationProofKind {
		return errors.New("unsupported aggregation serving evidence")
	}
	common := StartupEvidence(e)
	common.ProofKind = "startup-tls-initialized"
	return common.Validate(podUID, trustDigest, leafDigest, now)
}

// AggregationProof pins the Source installed on the aggregation TLS listener.
// Callers must serve it through a separately protected management endpoint.
type AggregationProof struct {
	proof *StartupProof
}

// NewAggregationProof requires only UCP server-auth identity, not component
// client-auth credentials, authorization mode, or deployment-engine evidence.
func NewAggregationProof(source *Source, namespace, podUID, expectedTrust string) (*AggregationProof, error) {
	if source == nil || namespace == "" || !validProofUUID(podUID) || !validFingerprint(expectedTrust) {
		return nil, errors.New("invalid aggregation serving proof configuration")
	}
	source.mu.Lock()
	if source.proofNamespace != "" || source.aggregationNamespace != "" && source.aggregationNamespace != namespace {
		source.mu.Unlock()
		return nil, errors.New("aggregation proof cannot reuse a component or another namespace's Source")
	}
	m, err := source.loadCurrent()
	if err == nil {
		err = source.validateAggregationIdentity(m, namespace)
	}
	if err == nil {
		source.aggregationNamespace = namespace
	}
	source.mu.Unlock()
	if err != nil {
		return nil, err
	}
	proof, err := NewStartupProof(source, podUID, expectedTrust)
	if err != nil {
		return nil, err
	}
	return &AggregationProof{proof: proof}, nil
}

func (s *Source) validateAggregationIdentity(m *material, namespace string) error {
	leaf := m.cert.Leaf
	if leaf.IsCA || leaf.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		return errors.New("invalid aggregation serving certificate usage")
	}
	chains, err := leaf.Verify(x509.VerifyOptions{Roots: m.pool, Intermediates: m.intermediates,
		CurrentTime: s.now(), DNSName: "ucp." + namespace + ".svc", KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
	if err != nil {
		return errors.New("invalid aggregation serving certificate chain or usage")
	}
	identity, err := identityFromChains(chains, namespace)
	if err != nil || identity.Component != "ucp" {
		return errors.New("invalid aggregation serving identity or key")
	}
	return nil
}

// Snapshot refreshes the same pinned Source used by the serving listener.
func (p *AggregationProof) Snapshot() (AggregationEvidence, error) {
	evidence, err := p.proof.Snapshot()
	if err != nil {
		return AggregationEvidence{}, err
	}
	evidence.ProofKind = aggregationProofKind
	return AggregationEvidence(evidence), nil
}

// ServeHTTP emits only public, bounded, non-cacheable serving evidence.
func (p *AggregationProof) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	serveStartupEvidence(w, r, func() (StartupEvidence, error) {
		evidence, err := p.Snapshot()
		return StartupEvidence(evidence), err
	})
}
