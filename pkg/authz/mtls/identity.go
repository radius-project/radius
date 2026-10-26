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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/radius-project/radius/pkg/authz"
)

// Components lists the Radius components that may hold an mTLS identity. The names
// match the Kubernetes Service names and the Certificates issued by the Helm chart.
var Components = []string{"ucp", "applications-rp", "dynamic-rp", "controller", "bicep-de"}

// minRSABits is the smallest RSA key accepted from a peer.
const minRSABits = 2048

// Identity is a verified Radius component identity.
type Identity struct {
	// Component is the component name, such as "applications-rp".
	Component string

	// Namespace is the Radius namespace.
	Namespace string
}

// String returns the canonical identity, "<component>.<namespace>.svc".
func (i Identity) String() string {
	return i.Component + "." + i.Namespace + ".svc"
}

// Error is a certificate failure with an authorization error code. Callers log Code;
// Reason is a short description that never contains key material.
type Error struct {
	// Code is authz.CodePeerCertificateInvalid or authz.CodeCertificateExpired.
	Code string

	// Reason describes the failure.
	Reason string

	// Err is the underlying error, if any.
	Err error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Reason, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Reason)
}

// Unwrap returns the underlying error.
func (e *Error) Unwrap() error {
	return e.Err
}

func peerError(reason string, err error) *Error {
	return &Error{Code: authz.CodePeerCertificateInvalid, Reason: reason, Err: err}
}

// PeerIdentity maps a verified peer chain to a Radius component identity. chain[0]
// must be the peer's leaf certificate. Every certificate in the chain must use an
// allowed key, and the leaf must have exactly one DNS SAN, the canonical
// "<component>.<namespace>.svc" name of a known component. The subject common name
// is ignored.
func PeerIdentity(chain []*x509.Certificate, namespace string) (Identity, error) {
	if len(chain) == 0 {
		return Identity{}, peerError("no peer certificate", nil)
	}
	for _, cert := range chain {
		if err := checkKey(cert); err != nil {
			return Identity{}, err
		}
	}

	sans := chain[0].DNSNames
	switch len(sans) {
	case 0:
		return Identity{}, peerError("certificate has no DNS SAN", nil)
	case 1:
		return identityFromSAN(sans[0], namespace)
	default:
		return Identity{}, peerError("certificate has more than one DNS SAN", nil)
	}
}

// identityFromSAN returns the identity named by san, which must be
// "<component>.<namespace>.svc" for a known component.
func identityFromSAN(san, namespace string) (Identity, error) {
	component, ok := strings.CutSuffix(strings.ToLower(san), "."+namespace+".svc")
	if !ok || !slices.Contains(Components, component) {
		return Identity{}, peerError(fmt.Sprintf("DNS SAN %q is not a Radius component in namespace %q", san, namespace), nil)
	}
	return Identity{Component: component, Namespace: namespace}, nil
}

// checkKey allows only ECDSA P-256 or P-384 and RSA keys of at least 2048 bits.
func checkKey(cert *x509.Certificate) error {
	switch key := cert.PublicKey.(type) {
	case *ecdsa.PublicKey:
		if key.Curve == elliptic.P256() || key.Curve == elliptic.P384() {
			return nil
		}
		return peerError(fmt.Sprintf("ECDSA curve %s is not allowed", key.Curve.Params().Name), nil)
	case *rsa.PublicKey:
		if key.N.BitLen() >= minRSABits {
			return nil
		}
		return peerError(fmt.Sprintf("RSA key size %d is below %d", key.N.BitLen(), minRSABits), nil)
	default:
		return peerError(fmt.Sprintf("key type %T is not allowed", cert.PublicKey), nil)
	}
}

// LocalIdentity returns this component's identity from the certificate in
// options.CertificateDirectory.
func LocalIdentity(options authz.Options) (Identity, error) {
	data, err := os.ReadFile(filepath.Join(options.CertificateDirectory, "tls.crt"))
	if err != nil {
		return Identity{}, fmt.Errorf("failed to read local certificate: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return Identity{}, errors.New("local certificate is not PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return Identity{}, fmt.Errorf("failed to parse local certificate: %w", err)
	}
	return PeerIdentity([]*x509.Certificate{cert}, options.Namespace)
}
