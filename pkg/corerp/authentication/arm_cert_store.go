// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package authentication

import (
	"sync"
)

// ArmCertStore stores active client certificates fetched from arm metadata endpoint
type ArmCertStore struct {
	ThumbprintMap sync.Map // maps from thumbprint -> Certificate
}

// newArmCertStore creates a new armstore to store the active arm client thumbprint
func newArmCertStore() *ArmCertStore {
	var syncMap sync.Map
	return &ArmCertStore{
		ThumbprintMap: syncMap,
	}
}

// storeCertificates stores the thumbprint fetched from arm metadata endpoint in memory
func (a *ArmCertStore) storeCertificates(certificates []Certificate) {
	for _, cert := range certificates {
		if _, ok := a.ThumbprintMap.Load(cert.Thumbprint); !ok {
			a.ThumbprintMap.Store(cert.Thumbprint, cert)
		}
	}
}

// getValidCertificates purges the thumbprints that are expired and stores the thumbprint that are active
func (a *ArmCertStore) getValidCertificates() ([]Certificate, error) {
	err := a.purgeOldCertificates()
	if err != nil {
		return nil, err
	}
	var validCertificates []Certificate
	a.ThumbprintMap.Range(func(k, v interface{}) bool {
		valid, err := v.(Certificate).certificateIsCurrent()
		if err != nil {
			return false
		}
		if valid {
			validCertificates = append(validCertificates, v.(Certificate))
		}
		return true
	})
	return validCertificates, nil
}

// purgeOldCertificates updates the cert store with active thumbprints
func (a *ArmCertStore) purgeOldCertificates() error {
	var certificates []Certificate
	a.ThumbprintMap.Range(func(k, v interface{}) bool {
		expired, err := v.(Certificate).certificateExpired()
		if err != nil {
			return false
		}
		if !expired {
			certificates = append(certificates, v.(Certificate))
		}
		return true
	})

	var validThumbprintMap sync.Map
	for _, cert := range certificates {
		validThumbprintMap.Store(cert.Thumbprint, cert)
	}
	a.ThumbprintMap = validThumbprintMap
	return nil
}
