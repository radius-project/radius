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
	"github.com/project-radius/radius/pkg/azure/tokencredentials"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/sdk"
	"github.com/project-radius/radius/pkg/ucp/credentials"
	"github.com/project-radius/radius/pkg/ucp/resources"
	ucp_provider "github.com/project-radius/radius/pkg/ucp/secret/provider"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

// Provider's config parameter need to match the values expected by Terraform
// https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs
const (
	AzureProviderName      = "azurerm"
	AzureFeaturesParam     = "features"
	AzureSubIDParam        = "subscription_id"
	AzureClientIDParam     = "client_id"
	AzureClientSecretParam = "client_secret"
	AzureTenantIDParam     = "tenant_id"
)

type azureProvider struct {
	ucpConn               sdk.Connection
	secretProviderOptions ucp_provider.SecretProviderOptions
}

// NewAzureProvider creates a new AzureProvider instance.
func NewAzureProvider(ucpConn sdk.Connection, secretProviderOptions ucp_provider.SecretProviderOptions) Provider {
	return &azureProvider{ucpConn: ucpConn, secretProviderOptions: secretProviderOptions}
}

// BuildConfig generates the Terraform provider configuration for Azure provider.
// https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs
func (p *azureProvider) BuildConfig(ctx context.Context, envConfig *recipes.Configuration) map[string]any {
	// features block is required for Azure provider even if it is empty
	// https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs#argument-reference
	azureConfig := map[string]any{
		AzureFeaturesParam: map[string]any{},
	}

	logger := ucplog.FromContextOrDiscard(ctx)
	if (envConfig == nil) || (envConfig.Providers == datamodel.Providers{}) || (envConfig.Providers.Azure == datamodel.ProvidersAzure{}) || envConfig.Providers.Azure.Scope == "" {
		logger.Info("Azure provider scope is not configured on the Environment, skipping Azure subscriptionID configuration.")
		return azureConfig
	}

	subscriptionID, _ := p.parseScope(ctx, envConfig.Providers.Azure.Scope)
	credentials := fetchAzureCredentials(ctx, p.getCredentialsProvider(ctx))

	return p.generateProviderConfigMap(ctx, azureConfig, credentials, subscriptionID)
}

func (p *azureProvider) generateProviderConfigMap(ctx context.Context, configMap map[string]any, credentials *credentials.AzureCredential, subscriptionID string) map[string]any {
	logger := ucplog.FromContextOrDiscard(ctx)
	if credentials != nil && credentials.ClientID != "" && credentials.TenantID != "" && credentials.ClientSecret != "" {
		configMap[AzureClientIDParam] = credentials.ClientID
		configMap[AzureClientSecretParam] = credentials.ClientSecret
		configMap[AzureTenantIDParam] = credentials.TenantID
	} else {
		logger.Info("Azure credentials provider is not configured on the Environment, skipping credentials configuration.")
	}

	return configMap
}

func (p *azureProvider) parseScope(ctx context.Context, scope string) (subscriptionID string, err error) {
	// logger := ucplog.FromContextOrDiscard(ctx)
	parsedScope, err := resources.Parse(scope)
	if err != nil {
		return "", v1.NewClientErrInvalidRequest(fmt.Sprintf("Invalid Azure provider scope %q is configured on the Environment, error parsing: %s", scope, err.Error()))
	}

	return parsedScope.FindScope(resources.SubscriptionsSegment), nil
}

func (p *azureProvider) getCredentialsProvider(ctx context.Context) *credentials.AzureCredentialProvider {
	logger := ucplog.FromContextOrDiscard(ctx)
	azureCredentialProvider, err := credentials.NewAzureCredentialProvider(ucp_provider.NewSecretProvider(p.secretProviderOptions), p.ucpConn, &tokencredentials.AnonymousCredential{})
	if err != nil {
		logger.Info(fmt.Sprintf("Error creating Azure credential provider, skipping credentials configuration. Err: %s ", err.Error()))
		return nil
	}

	return azureCredentialProvider
}

// fetchAzureCredentials Fetches Azure credentials from UCP. Returns nil if an error is received or the credentials are empty.
func fetchAzureCredentials(ctx context.Context, azureCredentialsProvider credentials.CredentialProvider[credentials.AzureCredential]) *credentials.AzureCredential {
	logger := ucplog.FromContextOrDiscard(ctx)
	credentials, err := azureCredentialsProvider.Fetch(context.Background(), credentials.AzureCloud, "default")
	if err != nil {
		logger.Info(fmt.Sprintf("Error fetching Azure credentials, skipping credentials configuration. Err: %s", err.Error()))
		return nil
	}

	return credentials
}
