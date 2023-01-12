// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
