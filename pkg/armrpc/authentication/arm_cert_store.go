/*
Copyright 2023 The Radius Authors.

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
