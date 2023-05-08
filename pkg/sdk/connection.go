/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package sdk

import (
	"net/http"
)

// Connection represents the configuration needed to connect to Radius. Use the functions
// in this package to create a connection.
type Connection interface {
	// Client returns an http.Client for communicating with Radius. This satisfies both the
	// autorest.Sender interface (autorest Track1 Go SDK) and policy.Transporter interface
	// (autorest Track2 Go SDK).
	Client() *http.Client

	// Endpoint returns the endpoint (aka. base URL) of the Radius API. This definitely includes
	// the URL scheme and authority, and may include path segments.
	Endpoint() string
}
