// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package authentication

import (
	"sync"
	"time"
)

// newArmCertStore creates a new armstore to store the active arm client thumbprint
func newArmCertStore() *armCertStore {
	var syncMap sync.Map
	return &armCertStore{
		thumbprintMap: syncMap,
	}
}

// storeCertificates stores the thumbprint fetched from arm metadata endpoint in memory
func (a *armCertStore) storeCertificates(certificates []certificate) {
	for _, cert := range certificates {
		if _, ok := a.thumbprintMap.Load(cert.Thumbprint); !ok {
			a.thumbprintMap.Store(cert.Thumbprint, cert)
		}
	}
}

// getValidCertificates purges the thumbprints that are expired and stores the thumbprint that are active
func (a *armCertStore) getValidCertificates() ([]certificate, error) {
	err := a.purgeOldCertificates()
	if err != nil {
		return nil, err
	}
	var validCertificates []certificate
	a.thumbprintMap.Range(func(k, v interface{}) bool {
		valid, err := v.(certificate).certificateIsCurrent()
		if err != nil {
			return false
		}
		if valid {
			validCertificates = append(validCertificates, v.(certificate))
		}
		return true
	})
	return validCertificates, nil
}

// purgeOldCertificates updates the cert store with active thumbprints
func (a *armCertStore) purgeOldCertificates() error {
	var certificates []certificate
	a.thumbprintMap.Range(func(k, v interface{}) bool {
		expired, err := v.(certificate).certificateExpired()
		if err != nil {
			return false
		}
		if !expired {
			certificates = append(certificates, v.(certificate))
		}
		return true
	})

	var validThumbprintMap sync.Map
	for _, cert := range certificates {
		validThumbprintMap.Store(cert.Thumbprint, cert)
	}
	a.thumbprintMap = validThumbprintMap
	return nil
}

// certificateIsCurrent verifies if a certificate has a valid startDate and is not expired
func (c certificate) certificateIsCurrent() (bool, error) {
	expired, err := c.certificateExpired()
	if err != nil {
		return false, err
	}
	hasStarted, err := c.certificateStarted()
	if err != nil {
		return false, err
	}
	if expired || !hasStarted {
		return false, nil
	}
	return true, nil
}

// certificateExpired verifies the expiry of a certificate
func (c certificate) certificateExpired() (bool, error) {
	if time.Now().Before(c.NotAfter) {
		return false, nil
	}
	return true, nil
}

// certificateStarted verfies the start time of a certificate
func (c certificate) certificateStarted() (bool, error) {
	if time.Now().Before(c.NotBefore) {
		return false, nil
	}
	return true, nil
}
