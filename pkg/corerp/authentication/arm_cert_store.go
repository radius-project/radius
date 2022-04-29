// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package authentication

import (
	"sync"
)

// ArmCertStore stores active client certificates fetched from arm metadata endpoint
type ArmCertStore sync.Map

// newArmCertStore creates a new armstore to store the active arm client thumbprint
func newArmCertStore() *ArmCertStore {
	return &ArmCertStore{}
}

// storeCertificates stores the thumbprint fetched from arm metadata endpoint in memory
func (a *ArmCertStore) storeCertificates(certificates []Certificate) {
	for _, cert := range certificates {
		if _, ok := (*sync.Map)(a).Load(cert.Thumbprint); !ok {
			(*sync.Map)(a).Store(cert.Thumbprint, cert)
		}
	}
}

// getValidCertificates purges the thumbprints that are expired and stores the thumbprint that are active
func (a *ArmCertStore) getValidCertificates() ([]Certificate, error) {
	a.purgeOldCertificates()
	var validCertificates []Certificate
	(*sync.Map)(a).Range(func(k, v interface{}) bool {
		valid := v.(Certificate).isValid()
		if valid {
			validCertificates = append(validCertificates, v.(Certificate))
		}
		return true
	})
	return validCertificates, nil
}

// purgeOldCertificates updates the cert store with active thumbprints
func (a *ArmCertStore) purgeOldCertificates() {
	var certificates []Certificate
	(*sync.Map)(a).Range(func(k, v interface{}) bool {
		expired := v.(Certificate).isExpired()
		if expired {
			certificates = append(certificates, v.(Certificate))
		}
		return true
	})
	for _, cert := range certificates {
		(*sync.Map)(a).Delete(cert.Thumbprint)
	}
}
