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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"slices"
)

// ServerConfig returns a TLS config for a server that requires a client certificate
// from the current CA bundle whose DNS SANs name a Radius component in namespace.
// The certificate and CA bundle are read for each handshake, so reloads take effect
// without a restart.
func (s *Source) ServerConfig(namespace string) *tls.Config {
	outer := &tls.Config{MinVersion: tls.VersionTLS12}
	outer.GetConfigForClient = func(*tls.ClientHelloInfo) (*tls.Config, error) {
		cert, err := s.certificate()
		if err != nil {
			return nil, err
		}

		// http.Server adds ALPN protocols to the outer config; keep them so HTTP/2 still works.
		nextProtos := slices.Clone(outer.NextProtos)
		if !slices.Contains(nextProtos, "http/1.1") {
			nextProtos = append(nextProtos, "http/1.1")
		}

		return &tls.Config{
			MinVersion:   tls.VersionTLS12,
			NextProtos:   nextProtos,
			Certificates: []tls.Certificate{*cert},
			ClientAuth:   tls.RequireAndVerifyClientCert,
			ClientCAs:    s.pool(),
			VerifyConnection: func(cs tls.ConnectionState) error {
				_, err := identityFromChains(cs.VerifiedChains, namespace)
				return err
			},
		}, nil
	}
	return outer
}

// ClientConfig returns a TLS config for a client that presents the current certificate
// and accepts only a server whose certificate is from the current CA bundle and whose
// only DNS SAN is the server name. The server name must be the canonical
// "<component>.<namespace>.svc" name of a Radius component, set either in the returned
// config or by the caller, such as http.Transport from the request URL.
func (s *Source) ClientConfig(namespace string) *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return s.certificate()
		},
		// The default verifier only supports a fixed RootCAs pool. VerifyConnection
		// performs the full chain and hostname check against the current CA bundle instead.
		InsecureSkipVerify: true, //nolint:gosec // verification is done in VerifyConnection.
		VerifyConnection: func(cs tls.ConnectionState) error {
			_, err := s.verifyServer(cs.PeerCertificates, cs.ServerName, namespace)
			return err
		},
	}
}

// verifyServer verifies a server chain against the current CA bundle and server name,
// then maps it to a component identity.
func (s *Source) verifyServer(certs []*x509.Certificate, serverName, namespace string) (Identity, error) {
	if len(certs) == 0 {
		return Identity{}, peerError("no peer certificate", nil)
	}
	if _, err := identityFromSAN(serverName, namespace); err != nil {
		return Identity{}, peerError(fmt.Sprintf("server name %q is not a canonical Radius component name", serverName), nil)
	}
	return s.verifyPeer(certs, serverName, x509.ExtKeyUsageServerAuth, namespace)
}

// verifyClient checks a client's certificate chain against the current CA bundle and
// returns its identity. Servers use it when the TLS handshake requests but does not
// verify a client certificate, as in dry-run mode.
func (s *Source) verifyClient(certs []*x509.Certificate, namespace string) (Identity, error) {
	if len(certs) == 0 {
		return Identity{}, peerError("no client certificate", nil)
	}
	return s.verifyPeer(certs, "", x509.ExtKeyUsageClientAuth, namespace)
}

func (s *Source) verifyPeer(certs []*x509.Certificate, dnsName string, usage x509.ExtKeyUsage, namespace string) (Identity, error) {
	intermediates := x509.NewCertPool()
	for _, cert := range certs[1:] {
		intermediates.AddCert(cert)
	}

	chains, err := certs[0].Verify(x509.VerifyOptions{
		Roots:         s.pool(),
		Intermediates: intermediates,
		DNSName:       dnsName,
		CurrentTime:   s.now(),
		KeyUsages:     []x509.ExtKeyUsage{usage},
	})
	if err != nil {
		return Identity{}, peerError("certificate verification failed", err)
	}
	return identityFromChains(chains, namespace)
}

// identityFromChains returns the identity from the first verified chain that passes
// PeerIdentity, or the first chain's error.
func identityFromChains(chains [][]*x509.Certificate, namespace string) (Identity, error) {
	if len(chains) == 0 {
		return Identity{}, peerError("no verified peer certificate", nil)
	}

	var firstErr error
	for _, chain := range chains {
		id, err := PeerIdentity(chain, namespace)
		if err == nil {
			return id, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return Identity{}, firstErr
}

type identityKey struct{}

// WithIdentity returns a copy of ctx that carries the caller's identity.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

// IdentityFromContext returns the caller's identity stored by WithIdentity.
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey{}).(Identity)
	return id, ok
}

// IdentityFromRequest returns the caller's identity from the request's verified client
// certificate chains. It requires a server using ServerConfig, so the chains were
// verified against the Radius CA bundle during the handshake.
func IdentityFromRequest(r *http.Request, namespace string) (Identity, error) {
	if r.TLS == nil {
		return Identity{}, peerError("request did not use TLS", nil)
	}
	return identityFromChains(r.TLS.VerifiedChains, namespace)
}
