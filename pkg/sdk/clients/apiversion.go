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

package clients

import (
	"context"
	"fmt"
	"strings"

	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/sdk"
	ucpv20231001preview "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

var (
	// DeploymentsClientAPIVersion is the API version of the UCP deployment client.
	DeploymentsClientAPIVersion = "2020-10-01"

	// DeploymentOperationsClientAPIVersion is the API version of the UCP deployment operations client.
	DeploymentOperationsClientAPIVersion = "2020-10-01"
)

// ResolveAPIVersion looks up a Radius resource type's supported API version in its provider summary.
func ResolveAPIVersion(ctx context.Context, connection sdk.Connection, id resources.ID) (string, error) {
	if !strings.HasPrefix(strings.ToLower(id.PlaneNamespace()), "radius/") {
		return "", fmt.Errorf("resource %q is not in a Radius plane", id.String())
	}

	client, err := ucpv20231001preview.NewResourceProvidersClient(&aztoken.AnonymousCredential{}, sdk.NewClientOptions(connection))
	if err != nil {
		return "", err
	}

	summary, err := client.GetProviderSummary(ctx, id.FindScope("radius"), id.ProviderNamespace(), nil)
	if err != nil {
		return "", err
	}

	_, shortType, _ := strings.Cut(id.Type(), "/")
	for name, resourceType := range summary.ResourceTypes {
		if !strings.EqualFold(name, shortType) || resourceType == nil {
			continue
		}

		if resourceType.DefaultAPIVersion != nil && *resourceType.DefaultAPIVersion != "" {
			if _, ok := resourceType.APIVersions[*resourceType.DefaultAPIVersion]; !ok {
				return "", fmt.Errorf("default API version %q for type %q is not advertised", *resourceType.DefaultAPIVersion, id.Type())
			}

			return *resourceType.DefaultAPIVersion, nil
		}

		selected := ""
		for version := range resourceType.APIVersions {
			if version != "" && (selected == "" || version < selected) {
				selected = version
			}
		}

		if selected != "" {
			return selected, nil
		}

		return "", fmt.Errorf("resource type %q has no supported API versions", id.Type())
	}

	return "", fmt.Errorf("resource type %q was not found in provider %q", id.Type(), id.ProviderNamespace())
}
