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
func (c Certificate) isValid() (bool, error) {
	if c.isExpired() || !c.isStarted() {
		return false, nil
	}
	return true, nil
}

// certificateExpired verifies the expiry of a certificate
func (c Certificate) isExpired() bool {
	if time.Now().Before(c.NotAfter) {
		return false
	}
	return true
}

// certificateStarted verfies the start time of a certificate
func (c Certificate) isStarted() bool {
	if time.Now().Before(c.NotBefore) {
		return false
	}
	return true
}
