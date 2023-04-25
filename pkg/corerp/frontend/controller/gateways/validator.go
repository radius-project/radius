// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
			newResource.Properties.TLS.MinimumProtocolVersion = datamodel.MinimumProtocolVersion12
		}
	}

	return nil, nil
}
