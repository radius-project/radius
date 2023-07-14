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

package providers

import (
	"context"
	"fmt"
	"strings"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

const (
	AzureProviderName = "azurerm"
)

type azureProvider struct{}

// NewAzureProvider creates a new AzureProvider instance.
func NewAzureProvider() Provider {
	return &azureProvider{}
}

// BuildConfig generates the Terraform provider configuration for Azure provider.
// https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs
func (p *azureProvider) BuildConfig(ctx context.Context, envConfig *recipes.Configuration) (map[string]any, error) {
	if (envConfig == nil) || (envConfig.Providers == datamodel.Providers{}) || (envConfig.Providers.Azure == datamodel.ProvidersAzure{}) || envConfig.Providers.Azure.Scope == "" {
		return nil, v1.NewClientErrInvalidRequest("Azure provider is required to be configured on the Environment to create Azure resources using Recipe")
	}

	subscriptionID, err := parseAzureScope(envConfig.Providers.Azure.Scope)
	if err != nil {
		return nil, err
	}

	// features block is required for Azure provider even if it is empty
	// https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs#argument-reference
	azureConfig := map[string]any{
		"subscription_id": subscriptionID,
		"features":        map[string]any{},
	}

	return azureConfig, nil
}

func parseAzureScope(scope string) (subscriptionID string, err error) {
	parsedScope, err := resources.Parse(scope)
	if err != nil {
		return "", v1.NewClientErrInvalidRequest(fmt.Sprintf("error parsing Azure scope %q: %s", scope, err.Error()))
	}

	for _, segment := range parsedScope.ScopeSegments() {
		if strings.EqualFold(segment.Type, resources.SubscriptionsSegment) {
			subscriptionID = segment.Name
		}
	}

	return subscriptionID, nil
}
