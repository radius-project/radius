// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package authentication

import "time"

// Certificate represents the client certificate fetched from arm metadata endpoint
type Certificate struct {
	Certificate string    `json:"certificate"`
	NotAfter    time.Time `json:"notAfter"`
	NotBefore   time.Time `json:"notBefore"`
	Thumbprint  string    `json:"thumbprint"`
}

// ClientCertificates stores the array of certificate returned from arm metadata endpoint
type clientCertificates struct {
	ClientCertificates []Certificate `json:"clientCertificates"`
}

// certificateIsCurrent verifies if a certificate has a valid startDate and is not expired
func (c Certificate) certificateIsCurrent() (bool, error) {
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
func (c Certificate) certificateExpired() (bool, error) {
	if time.Now().Before(c.NotAfter) {
		return false, nil
	}
	return true, nil
}

// certificateStarted verfies the start time of a certificate
func (c Certificate) certificateStarted() (bool, error) {
	if time.Now().Before(c.NotBefore) {
		return false, nil
	}
	return true, nil
}
