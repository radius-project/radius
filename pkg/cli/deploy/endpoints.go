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
