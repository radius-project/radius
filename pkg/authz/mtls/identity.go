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
	"fmt"
	"net/url"

	"github.com/radius-project/radius/pkg/authz"
)

const (
	// DefaultTrustDomain is the SPIFFE trust domain of Radius server certificates when
	// authz.Options.TrustDomain is unset. It matches the chart value global.rbac.spiffe.trustDomain.
	DefaultTrustDomain = "radius.internal"

	// minRSABits is the smallest RSA key accepted from a peer.
	minRSABits = 2048
)

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

// SPIFFEID returns the SPIFFE ID that csi-driver-spiffe puts in the server certificate of
// the pod running as serviceAccount in namespace: spiffe://<trustDomain>/ns/<namespace>/sa/<serviceAccount>.
func SPIFFEID(trustDomain, namespace, serviceAccount string) string {
	return (&url.URL{Scheme: "spiffe", Host: trustDomain, Path: "/ns/" + namespace + "/sa/" + serviceAccount}).String()
}

// checkSPIFFEID checks that leaf has exactly one URI SAN and that it is expected.
// DNS SANs and the subject common name are ignored.
func checkSPIFFEID(leaf *x509.Certificate, expected string) error {
	switch len(leaf.URIs) {
	case 0:
		return peerError("server certificate has no URI SAN", nil)
	case 1:
	default:
		return peerError("server certificate has more than one URI SAN", nil)
	}
	if got := leaf.URIs[0].String(); got != expected {
		return peerError(fmt.Sprintf("server certificate SPIFFE ID %q is not %q", got, expected), nil)
	}
	return nil
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
