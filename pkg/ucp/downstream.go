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

package ucp

import (
	"net/http"

	"github.com/radius-project/radius/pkg/authz/mtls"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// mtlsDownstreams are the resource providers that require UCP's client certificate when
// the authorization mode is not off.
var mtlsDownstreams = []string{"applications-rp", "dynamic-rp"}

// NewDownstreamTransport returns the transport UCP uses to forward requests to resource
// providers. When the authorization mode is not off, requests to applications-rp and
// dynamic-rp use https and present UCP's certificate, even when the registered location
// is an http URL. With the external provider, those providers and the deployment engine
// instead receive their own projected token over operator-encrypted HTTP.
// Other downstreams are unchanged.
func NewDownstreamTransport(config *Config) (http.RoundTripper, error) {
	downstreams := mtlsDownstreams
	if config.Authorization.EffectiveMTLSProvider() == "external" {
		downstreams = []string{"applications-rp", "dynamic-rp", "bicep-de"}
	}
	transport, err := mtls.NewUpgradeTransport(config.Authorization, http.DefaultTransport, downstreams)
	if err != nil {
		return nil, err
	}
	return otelhttp.NewTransport(transport), nil
}
