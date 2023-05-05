// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deploy

import (
	"context"

	"github.com/project-radius/radius/pkg/cli/clients"
	ucpresources "github.com/project-radius/radius/pkg/ucp/resources"
)

type PublicEndpoint struct {
	Resource ucpresources.ID
	Endpoint string
}

// # Function Explanation
// 
//	FindPublicEndpoints iterates through the resources in the given DeploymentResult and attempts to retrieve the public 
//	endpoint for each resource using the DiagnosticsClient. If an endpoint is found, it is added to the list of 
//	PublicEndpoints. If an error occurs during the retrieval, it is returned to the caller.
func FindPublicEndpoints(ctx context.Context, diag clients.DiagnosticsClient, result clients.DeploymentResult) ([]PublicEndpoint, error) {
	endpoints := []PublicEndpoint{}
	for _, resource := range result.Resources {
		endpoint, err := diag.GetPublicEndpoint(ctx, clients.EndpointOptions{ResourceID: resource})
		if err != nil {
			return nil, err
		}

		if endpoint == nil {
			continue
		}

		endpoints = append(endpoints, PublicEndpoint{Resource: resource, Endpoint: *endpoint})
	}

	return endpoints, nil
}
