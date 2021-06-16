// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package certs

import (
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/fullsailor/pkcs7"
)

// Validate validates the client-certificate we expect from the X-ARR-ClientCert header
func Validate(header string) error {
	bytes, err := base64.StdEncoding.DecodeString(header)
	if err != nil {
		return fmt.Errorf("failed to decode base64 certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	// the certs we're working with generally have intermediates that need to be validated
	next := cert
	intermediates := x509.NewCertPool()
	for !isSelfSigned(next) && len(next.IssuingCertificateURL) > 0 {
		c, err := fetchCert(next.IssuingCertificateURL[0])
		if err != nil {
			return fmt.Errorf("failed to download intermediate certificate: %w", err)
		}

		intermediates.AddCert(c)
		next = c
	}

	_, err = cert.Verify(x509.VerifyOptions{
		DNSName:       "customproviders.authentication.metadata.management.azure.com",
		Intermediates: intermediates,
	})

	return err
}

func fetchCert(url string) (*x509.Certificate, error) {
	resp, err := http.Get(url)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}

	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	cert, err := x509.ParseCertificate(bytes)
	if err == nil {
		return cert, nil
	}

	p, err := pkcs7.Parse(bytes)
	if err == nil {
		return p.Certificates[0], nil
	}

	return nil, errors.New("failed to parse certificate")
}

func isSelfSigned(cert *x509.Certificate) bool {
	return cert.CheckSignatureFrom(cert) == nil
}
