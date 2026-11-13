// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authzcheck

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFrontProxyRequiresExplicitClientAuth(t *testing.T) {
	t.Parallel()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "request-header-ca"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &key.PublicKey, key)
	require.NoError(t, err)
	ca, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	config := &FrontProxyConfig{Roots: roots, AllowedNames: []string{"front-proxy-client"}}
	for _, spec := range []struct {
		name   string
		usages []x509.ExtKeyUsage
		valid  bool
	}{
		{"missing EKU", nil, false},
		{"any EKU", []x509.ExtKeyUsage{x509.ExtKeyUsageAny}, false},
		{"server only", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, false},
		{"client", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, true},
		{"client and server", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}, true},
	} {
		t.Run(spec.name, func(t *testing.T) {
			leaf := &x509.Certificate{
				SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "front-proxy-client"},
				NotBefore: ca.NotBefore, NotAfter: ca.NotAfter,
				KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: spec.usages,
			}
			der, err := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, key)
			require.NoError(t, err)
			cert, err := x509.ParseCertificate(der)
			require.NoError(t, err)
			err = config.Verify([]*x509.Certificate{cert})
			if spec.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
