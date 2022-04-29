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
func (c Certificate) isValid() bool {
	if c.isExpired() || !c.isStarted() {
		return false
	}
	return true
}

// certificateExpired verifies the expiry of a certificate
func (c Certificate) isExpired() bool {
	return !time.Now().Before(c.NotAfter)
}

// certificateStarted verfies the start time of a certificate
func (c Certificate) isStarted() bool {
	return !time.Now().Before(c.NotBefore)
}
