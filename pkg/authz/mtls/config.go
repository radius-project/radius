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
	"crypto/tls"
	"crypto/x509"
	"slices"
	"time"
)

// ServerConfig returns a TLS config for a server that presents the current certificate
// and does not request a client certificate; callers authenticate with a ServiceAccount
// token instead. The certificate is read for each handshake, so reloads take effect
// without a restart.
func (s *Source) ServerConfig() *tls.Config {
	return s.ServerConfigFrom(&tls.Config{ClientAuth: tls.NoClientCert})
}

// ServerConfigFrom returns a TLS config that presents the current certificate and keeps the
// other settings of base, such as ClientAuth and ClientCAs for a request-header proxy. A nil
// base requests no client certificate. Any certificates in base are replaced, and the minimum
// version is at least TLS 1.2. The certificate is read for each handshake, so reloads take
// effect without a restart.
func (s *Source) ServerConfigFrom(base *tls.Config) *tls.Config {
	if base == nil {
		base = &tls.Config{ClientAuth: tls.NoClientCert}
	}
	base = base.Clone()
	base.Certificates = nil
	base.GetCertificate = nil
	base.GetConfigForClient = nil
	if base.MinVersion < tls.VersionTLS12 {
		base.MinVersion = tls.VersionTLS12
	}

	outer := base.Clone()
	outer.GetConfigForClient = func(*tls.ClientHelloInfo) (*tls.Config, error) {
		m, err := s.load()
		if err != nil {
			return nil, err
		}

		// http.Server adds ALPN protocols to the outer config; keep them so HTTP/2 still works.
		nextProtos := slices.Clone(outer.NextProtos)
		if !slices.Contains(nextProtos, "http/1.1") {
			nextProtos = append(nextProtos, "http/1.1")
		}

		config := base.Clone()
		config.NextProtos = nextProtos
		config.Certificates = []tls.Certificate{*m.cert}
		return config, nil
	}
	return outer
}

// clientConfig returns a TLS config for a client that presents no certificate and accepts
// only a server whose certificate chains to the current CA bundle for server
// authentication and whose only SAN is the URI SAN spiffeID. The server name is used for SNI
// only; a certificate with any other SAN, such as a DNS name, is rejected.
//
// The config is bound to the CA bundle loaded when clientConfig is called. Call it for each
// new connection, for example from http.Transport.DialTLSContext, so CA reloads take effect.
// If verified is not nil, it receives the verified chain after a successful check.
func (s *Source) clientConfig(spiffeID string, verified func([]*x509.Certificate)) (*tls.Config, error) {
	m, err := s.load()
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		// The standard verifier matches the server name against DNS SANs, but Radius server
		// certificates carry only a SPIFFE URI SAN. VerifyConnection checks the chain against
		// the Radius CA bundle and the SPIFFE ID instead, on every handshake.
		InsecureSkipVerify: true, //nolint:gosec // The chain and SPIFFE ID are verified in VerifyConnection.
		VerifyConnection: func(cs tls.ConnectionState) error {
			chain, err := verifyServer(m.pool, cs.PeerCertificates, spiffeID, s.now())
			if err != nil {
				return err
			}
			if verified != nil {
				verified(chain)
			}
			return nil
		},
	}, nil
}

// verifyServer checks a server's certificate chain against pool and its SPIFFE ID, and
// returns the verified chain. Every certificate in the chain must use an allowed key.
func verifyServer(pool *x509.CertPool, certs []*x509.Certificate, spiffeID string, now time.Time) ([]*x509.Certificate, error) {
	if len(certs) == 0 {
		return nil, peerError("server presented no certificate", nil)
	}

	intermediates := x509.NewCertPool()
	for _, cert := range certs[1:] {
		intermediates.AddCert(cert)
	}

	chains, err := certs[0].Verify(x509.VerifyOptions{
		Roots:         pool,
		Intermediates: intermediates,
		CurrentTime:   now,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	if err != nil {
		return nil, peerError("server certificate verification failed", err)
	}
	if err := checkSPIFFEID(certs[0], spiffeID); err != nil {
		return nil, err
	}

	var firstErr error
	for _, chain := range chains {
		var keyErr error
		for _, cert := range chain {
			if keyErr = checkKey(cert); keyErr != nil {
				break
			}
		}
		if keyErr == nil {
			return chain, nil
		}
		if firstErr == nil {
			firstErr = keyErr
		}
	}
	return nil, firstErr
}
