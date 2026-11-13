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

package helm

import (
	"bytes"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseCertificateBundlePEM(t *testing.T) {
	ca, other := newTestCA(t, "selected"), newTestCA(t, "old")
	expired := *other.cert
	expired.NotBefore, expired.NotAfter = time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour)
	der, err := x509.CreateCertificate(rand.Reader, &expired, &expired, &other.key.PublicKey, other.key)
	require.NoError(t, err)
	old := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	leafPEM := issueUCPCertificate(t, ca)
	for _, tt := range []struct {
		name  string
		input []byte
		count int
		err   string
	}{
		{"empty", nil, 0, "empty"},
		{"whitespace", []byte(" \t\r\n"), 0, "empty"},
		{"single", ca.pem, 1, ""},
		{"CRLF", bytes.ReplaceAll(ca.pem, []byte("\n"), []byte("\r\n")), 1, ""},
		{"historical overlap", append(bytes.Clone(ca.pem), old...), 2, ""},
		{"duplicates preserved", append(bytes.Clone(ca.pem), ca.pem...), 2, ""},
		{"leaf is lexically valid", leafPEM, 1, ""},
		{"leading junk", append([]byte("junk\n"), ca.pem...), 0, "expected"},
		{"trailing junk", append(bytes.Clone(ca.pem), []byte("junk")...), 0, "expected"},
		{"interblock junk", append(append(bytes.Clone(ca.pem), []byte("junk\n")...), other.pem...), 0, "expected"},
		{"key after cert", append(bytes.Clone(ca.pem), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("key")})...), 0, "expected"},
		{"header", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Headers: map[string]string{"Comment": "not permitted"}, Bytes: ca.cert.Raw}), 0, "headers"},
		{"broken first block before valid cert", append([]byte("-----BEGIN CERTIFICATE-----\n!!!!\n"), ca.pem...), 0, "malformed"},
		{"bad base64", []byte("-----BEGIN CERTIFICATE-----\n!!!!\n-----END CERTIFICATE-----"), 0, "non-base64"},
		{"invalid DER", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("not DER")}), 0, "invalid X.509"},
		{"unicode whitespace", append([]byte("\u00a0"), ca.pem...), 0, "expected"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			certs, err := parseCertificateBundlePEM(tt.input)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				require.Nil(t, certs)
				return
			}
			require.NoError(t, err)
			require.Len(t, certs, tt.count)
			if tt.count == 2 {
				require.Equal(t, ca.cert.Raw, certs[0].Raw)
			}
		})
	}
}
