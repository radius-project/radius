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

	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
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
func (p *azureProvider) BuildConfig(ctx context.Context, envConfig *recipes.Configuration) map[string]any {
	// features block is required for Azure provider even if it is empty
	// https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs#argument-reference
	azureConfig := map[string]any{
		"features": map[string]any{},
	}

	logger := ucplog.FromContextOrDiscard(ctx)
	if (envConfig == nil) || (envConfig.Providers == datamodel.Providers{}) || (envConfig.Providers.Azure == datamodel.ProvidersAzure{}) || envConfig.Providers.Azure.Scope == "" {
		logger.Info("Azure provider scope is not configured on the Environment, skipping Azure subscriptionID configuration.")
		return azureConfig
	}

	subscriptionID := parseAzureScope(ctx, envConfig.Providers.Azure.Scope)
	if subscriptionID != "" {
		azureConfig["subscription_id"] = subscriptionID
	}

	return azureConfig
}

func parseAzureScope(ctx context.Context, scope string) (subscriptionID string) {
	logger := ucplog.FromContextOrDiscard(ctx)
	parsedScope, err := resources.Parse(scope)
	if err != nil {
		logger.Info(fmt.Sprintf("Invalid Azure provider scope is configured on the Environment, error parsing: %s", err.Error()))
		return ""
	}

	for _, segment := range parsedScope.ScopeSegments() {
		if strings.EqualFold(segment.Type, resources.SubscriptionsSegment) {
			subscriptionID = segment.Name
		}
	}

	return subscriptionID
}
