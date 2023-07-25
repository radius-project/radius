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

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

const (
	AzureProviderName = "azurerm"
)

type azureProvider struct{}

// # Function Explanation
//
// NewAzureProvider creates a new AzureProvider instance.
func NewAzureProvider() Provider {
	return &azureProvider{}
}

// BuildConfig generates the Terraform provider configuration for Azure provider. It checks if the environment
// configuration contains an Azure provider scope and if so, parses the scope to get the subscriptionID and adds
// it to the config map. If the scope is invalid, an error is returned.
// https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs
func (p *azureProvider) BuildConfig(ctx context.Context, envConfig *recipes.Configuration) (map[string]any, error) {
	// features block is required for Azure provider even if it is empty
	// https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs#argument-reference
	config := map[string]any{
		"features": map[string]any{},
	}

	logger := ucplog.FromContextOrDiscard(ctx)
	if (envConfig == nil) || (envConfig.Providers == datamodel.Providers{}) || (envConfig.Providers.Azure == datamodel.ProvidersAzure{}) || envConfig.Providers.Azure.Scope == "" {
		logger.Info("Azure provider scope is not configured on the Environment, skipping Azure subscriptionID configuration.")
		return config, nil
	}

	subscriptionID, err := parseAzureScope(envConfig.Providers.Azure.Scope)
	if err != nil {
		return nil, err
	}
	if subscriptionID == "" {
		return nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("Invalid Azure provider scope %q is configured on the Environment, subscriptionID is required in the scope", envConfig.Providers.Azure.Scope))
	}
	config["subscription_id"] = subscriptionID

	return config, nil
}

// parseAzureScope parses an Azure provider scope and returns the associated subscription id
// Example scope: /subscriptions/test-sub/resourceGroups/test-rg
func parseAzureScope(scope string) (string, error) {
	parsedScope, err := resources.Parse(scope)
	if err != nil {
		return "", v1.NewClientErrInvalidRequest(fmt.Sprintf("Invalid Azure provider scope %q is configured on the Environment, error parsing: %s", scope, err.Error()))
	}

	return parsedScope.FindScope(resources.SubscriptionsSegment), nil
}
