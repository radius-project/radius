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
	"os"

	"github.com/radius-project/radius/pkg/authz"
)

// ProcessStartupProof initializes the actual shared component TLS Source before
// services start. Proof mode requires an existing management listener; it never
// enables metrics or weakens an application's mTLS listener.
func ProcessStartupProof(options authz.Options, managementAvailable bool) (*StartupProof, error) {
	proof, err := configuredStartupProof(options)
	if err != nil {
		return nil, err
	}
	if proof != nil && !managementAvailable {
		return nil, errors.New("TLS startup proof requires an enabled metrics listener on a fixed port")
	}
	return proof, nil
}

func configuredStartupProof(options authz.Options) (*StartupProof, error) {
	switch os.Getenv("UCP_MTLS_STARTUP_PROOF_ENABLED") {
	case "", "false":
		if os.Getenv("UCP_MTLS_EXPECTED_TRUST_SHA256") != "" || os.Getenv("UCP_MTLS_POD_UID") != "" {
			return nil, errors.New("TLS startup proof settings require proof mode")
		}
		return nil, nil
	case "true":
	default:
		return nil, errors.New("TLS startup proof enabled setting must be true or false")
	}
	digest := os.Getenv("UCP_MTLS_EXPECTED_TRUST_SHA256")
	podUID := os.Getenv("UCP_MTLS_POD_UID")
	if options.EffectiveMode() == authz.ModeOff || options.EffectiveMTLSProvider() != "radius" || options.CertificateDirectory == "" ||
		options.Namespace == "" || !validFingerprint(digest) || !validProofUUID(podUID) {
		return nil, errors.New("TLS startup proof requires enabled component mTLS, an expected trust digest and a pod UID")
	}
	source, err := sharedSource(options.CertificateDirectory)
	if err != nil {
		return nil, errors.New("cannot initialize component TLS startup proof")
	}
	if _, err := source.Reload(); err != nil {
		return nil, errors.New("component TLS startup material is unavailable")
	}
	if err := source.bindProofNamespace(options.Namespace); err != nil {
		return nil, err
	}
	return NewStartupProof(source, podUID, digest)
}

func (s *Source) bindProofNamespace(namespace string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.aggregationNamespace != "" {
		return errors.New("component proof cannot reuse an aggregation serving Source")
	}
	if s.proofNamespace != "" && s.proofNamespace != namespace {
		return errors.New("TLS startup proof namespace cannot change within a process")
	}
	m, err := s.loadCurrent()
	if err != nil {
		return errors.New("component TLS startup material is unavailable")
	}
	if err := s.validateProofIdentity(m, namespace); err != nil {
		return err
	}
	s.proofNamespace = namespace
	return nil
}

func (s *Source) validateProofIdentity(m *material, namespace string) error {
	leaf := m.cert.Leaf
	if leaf.IsCA || leaf.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		return errors.New("invalid component TLS startup certificate usage")
	}
	for _, usage := range []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth} {
		chains, err := leaf.Verify(x509.VerifyOptions{
			Roots: m.pool, Intermediates: m.intermediates, CurrentTime: s.now(),
			KeyUsages: []x509.ExtKeyUsage{usage},
		})
		if err != nil {
			return errors.New("invalid component TLS startup certificate chain or usage")
		}
		if _, err := identityFromChains(chains, namespace); err != nil {
			return errors.New("invalid component TLS startup identity or key")
		}
	}
	return nil
}

func clientSource(options authz.Options) (*Source, error) {
	proof, err := configuredStartupProof(options)
	if err != nil {
		return nil, err
	}
	if proof != nil {
		return proof.source, nil
	}
	return sharedSource(options.CertificateDirectory)
}

func listenerSource(options authz.Options) (*Source, error) {
	proof, err := configuredStartupProof(options)
	if err != nil {
		return nil, err
	}
	if proof != nil {
		return proof.source, nil
	}
	return NewSource(options.CertificateDirectory)
}
