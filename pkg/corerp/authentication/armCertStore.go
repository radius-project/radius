// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package authentication

import "time"

func newArmCertStore() *armCertStore {
	return &armCertStore{
		thumbprintMap: make(map[string]Certificate),
	}
}
func (self *armCertStore) storeCertificates(certificates []Certificate) {
	self.mutex.Lock()
	defer self.mutex.Unlock()
	for _, cert := range certificates {
		if _, ok := self.thumbprintMap[cert.Thumbprint]; !ok {
			self.thumbprintMap[cert.Thumbprint] = cert
		}
	}
}
func (self *armCertStore) getValidCertificates() ([]Certificate, error) {
	self.mutex.Lock()
	defer self.mutex.Unlock()
	err := self.purgeOldCertificates()
	if err != nil {
		return nil, err
	}
	var validCertificates []Certificate
	for _, cert := range self.thumbprintMap {
		valid, err := certificateIsCurrent(cert)
		if err != nil {
			return nil, err
		}
		if valid {
			validCertificates = append(validCertificates, cert)
		}
	}
	return validCertificates, nil
}

func (self *armCertStore) purgeOldCertificates() error {
	var certificates []Certificate
	for _, cert := range self.thumbprintMap {
		expired, err := certificateExpired(cert)
		if err != nil {
			return err
		}
		if !expired {
			certificates = append(certificates, cert)
		}
	}
	validThumbprintMap := make(map[string]Certificate)
	for _, cert := range certificates {
		validThumbprintMap[cert.Thumbprint] = cert
	}
	self.thumbprintMap = validThumbprintMap
	return nil
}

func certificateIsCurrent(certificate Certificate) (bool, error) {
	expired, err := certificateExpired(certificate)
	if err != nil {
		return false, err
	}
	hasStarted, err := certificateStarted(certificate)
	if err != nil {
		return false, err
	}
	if expired || !hasStarted {
		return false, nil
	}
	return true, nil
}

func certificateExpired(certificate Certificate) (bool, error) {
	notAfter, err := time.Parse(ArmTimeFormat, certificate.NotAfter)
	if err != nil {
		return false, err
	}
	if time.Now().Before(notAfter) {
		return false, nil
	}
	return true, nil
}

func certificateStarted(certificate Certificate) (bool, error) {
	notBefore, err := time.Parse(ArmTimeFormat, certificate.NotBefore)
	if err != nil {
		return false, err
	}
	if time.Now().Before(notBefore) {
		return false, nil
	}
	return true, nil
}
