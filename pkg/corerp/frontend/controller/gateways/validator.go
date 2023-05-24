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

package gateways

import (
	"context"

	"github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
)

// ValidateAndMutateRequest validates and mutates the incoming request.
func ValidateAndMutateRequest(ctx context.Context, newResource *datamodel.Gateway, oldResource *datamodel.Gateway, options *controller.Options) (rest.Response, error) {
	if newResource.Properties.TLS != nil {
		// If SSL Passthrough and TLS termination are both configured, then report an error
		if newResource.Properties.TLS.SSLPassthrough && newResource.Properties.TLS.CertificateFrom != "" {
			return rest.NewBadRequestResponse("Only one of $.properties.tls.certificateFrom and $.properties.tls.sslPassthrough can be specified at a time."), nil
		}

		// If TLS protocol version is set, then certificateFrom must be set
		if newResource.Properties.TLS.MinimumProtocolVersion != "" && newResource.Properties.TLS.CertificateFrom == "" {
			return rest.NewBadRequestResponse("Field $.properties.tls.certificateFrom is required when $.properties.tls.minimumProtocolVersion is set."), nil
		}

		// TLS protocol version defaults to 1.2
		if newResource.Properties.TLS.MinimumProtocolVersion == "" {
			newResource.Properties.TLS.MinimumProtocolVersion = datamodel.TLSMinVersion12
		}
	}

	return nil, nil
}
