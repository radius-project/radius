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
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

// parseCertificateBundlePEM parses the entire input, preserving order and duplicates.
// It makes no CA, validity, trust, or lifecycle decisions, including for historical overlap.
func parseCertificateBundlePEM(bundle []byte) ([]*x509.Certificate, error) {
	const begin, end = "-----BEGIN CERTIFICATE-----", "-----END CERTIFICATE-----"
	var certs []*x509.Certificate
	for remaining := bytes.Trim(bundle, " \t\r\n"); len(remaining) > 0; remaining = bytes.Trim(remaining, " \t\r\n") {
		index := len(certs) + 1
		if !bytes.HasPrefix(remaining, []byte(begin+"\n")) && !bytes.HasPrefix(remaining, []byte(begin+"\r\n")) {
			return nil, fmt.Errorf("certificate bundle block %d: expected a headerless CERTIFICATE PEM block; non-certificate data is not allowed", index)
		}
		stop := bytes.Index(remaining, []byte(end))
		if stop < 0 {
			return nil, fmt.Errorf("certificate bundle block %d: missing PEM end marker", index)
		}
		body := remaining[len(begin):stop]
		for _, b := range body {
			if !(b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '+' || b == '/' || b == '=' || b == '\r' || b == '\n') {
				return nil, fmt.Errorf("certificate bundle block %d: PEM headers, malformed blocks and non-base64 data are not allowed", index)
			}
		}
		encoded := remaining[:stop+len(end)]
		block, rest := pem.Decode(encoded)
		if block == nil || len(rest) != 0 || block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			return nil, fmt.Errorf("certificate bundle block %d: malformed CERTIFICATE PEM", index)
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("certificate bundle block %d: invalid X.509 DER: %w", index, err)
		}
		certs = append(certs, cert)
		remaining = remaining[len(encoded):]
		if len(remaining) > 0 && remaining[0] != '\n' && remaining[0] != '\r' && remaining[0] != ' ' && remaining[0] != '\t' {
			return nil, fmt.Errorf("certificate bundle block %d: trailing data after PEM end marker", index)
		}
	}
	if len(certs) == 0 {
		return nil, fmt.Errorf("certificate bundle is empty")
	}
	return certs, nil
}
