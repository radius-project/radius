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

package sdk

import (
	"fmt"
	"net/http"
	"net/url"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

var _ Connection = (*directConnection)(nil)

// directConnection represents a connection to a Radius API endpoint with no authentication
// or intermediate systems. This is mostly used for test scenarios.
type directConnection struct {
	endpoint string
}

// NewDirectConnection parses the given endpoint string and returns a direct connection if the endpoint uses the http or
// https scheme, otherwise it returns an error.
func NewDirectConnection(endpoint string) (Connection, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to parse endpoint %q: %w", endpoint, err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("the endpoint must use the http or https scheme (got %q)", endpoint)
	}

	return &directConnection{
		endpoint: endpoint,
	}, nil
}

// Client returns an http.Client for communicating with Radius. This satisfies both the
// autorest.Sender interface (autorest Track1 Go SDK) and policy.Transporter interface
// (autorest Track2 Go SDK).
func (c *directConnection) Client() *http.Client {
	return &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}
}

// Endpoint returns the endpoint (aka. base URL) of the Radius API. This definitely includes
// the URL scheme and authority, and may include path segments.
func (c *directConnection) Endpoint() string {
	return c.endpoint
}
