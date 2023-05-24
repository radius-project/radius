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
	return !c.isExpired() && c.isStarted()
}

// certificateExpired verifies the expiry of a certificate
func (c Certificate) isExpired() bool {
	return !time.Now().Before(c.NotAfter)
}

// certificateStarted verfies the start time of a certificate
func (c Certificate) isStarted() bool {
	return !time.Now().Before(c.NotBefore)
}
