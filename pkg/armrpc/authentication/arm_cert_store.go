// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package authentication

import (
	"sync"
)

// ArmCertStore stores active client certificates fetched from arm metadata endpoint
var ArmCertStore sync.Map

// storeCertificates stores the thumbprint fetched from arm metadata endpoint in memory
func storeCertificates(certificates []Certificate) {
	for _, cert := range certificates {
		ArmCertStore.LoadOrStore(cert.Thumbprint, cert)
	}
}

// getValidCertificates purges the thumbprints that are expired and stores the thumbprint that are active
func getValidCertificates() []Certificate {
	purgeOldCertificates()
	var validCertificates []Certificate
	ArmCertStore.Range(func(k, v any) bool {
		valid := v.(Certificate).isValid()
		if valid {
			validCertificates = append(validCertificates, v.(Certificate))
		}
		return true
	})
	return validCertificates
}

// purgeOldCertificates updates the cert store with active thumbprints
func purgeOldCertificates() {
	var certificates []Certificate
	ArmCertStore.Range(func(k, v any) bool {
		expired := v.(Certificate).isExpired()
		if expired {
			certificates = append(certificates, v.(Certificate))
		}
		return true
	})
	for _, cert := range certificates {
		ArmCertStore.Delete(cert.Thumbprint)
	}
}
