/*
Copyright 2026 The Radius Authors.

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

package installer

// JobMessage is the payload sent through the installer queue.
//
// SECURITY NOTE: This message may contain sensitive data (AuthHeader, ClientKey).
// The queue should be treated as containing secrets:
// - Ensure queue storage is appropriately secured
// - Avoid logging full message contents
// - Consider encryption at rest if using persistent queues
// Future improvement: store secrets in a secret store and pass references instead.
type JobMessage struct {
	Operation  Operation `json:"operation"`
	Version    string    `json:"version"`
	SourceURL  string    `json:"sourceUrl,omitempty"`
	Checksum   string    `json:"checksum,omitempty"`
	CABundle   string    `json:"caBundle,omitempty"`
	AuthHeader string    `json:"authHeader,omitempty"` // SENSITIVE: may contain bearer tokens
	ClientCert string    `json:"clientCert,omitempty"`
	ClientKey  string    `json:"clientKey,omitempty"` // SENSITIVE: contains private key material
	ProxyURL   string    `json:"proxyUrl,omitempty"`
	Purge      bool      `json:"purge,omitempty"` // Remove metadata after uninstall
}
