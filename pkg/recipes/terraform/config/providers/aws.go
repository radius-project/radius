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
	"errors"
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

const (
	AWSProviderName = "aws"
)

type awsProvider struct {
	ucpConn               *sdk.Connection
	secretProviderOptions ucp_provider.SecretProviderOptions
}

// NewAWSProvider creates a new AWSProvider instance.
func NewAWSProvider(ucpConn *sdk.Connection, secretProviderOptions ucp_provider.SecretProviderOptions) Provider {
	return &awsProvider{ucpConn: ucpConn, secretProviderOptions: secretProviderOptions}
}

// BuildConfig generates the Terraform provider configuration for AWS provider.
// https://registry.terraform.io/providers/hashicorp/aws/latest/docs
func (p *awsProvider) BuildConfig(ctx context.Context, envConfig *recipes.Configuration) map[string]any {
	logger := ucplog.FromContextOrDiscard(ctx)
	awsConfig := make(map[string]any)
	if (envConfig == nil) || (envConfig.Providers == datamodel.Providers{}) || (envConfig.Providers.AWS == datamodel.ProvidersAWS{}) || envConfig.Providers.AWS.Scope == "" {
		logger.Info("AWS provider scope is not configured on the Environment, skipping AWS region configuration.")
		return awsConfig
	}

	region, _ := parseAWSScope(envConfig.Providers.AWS.Scope)
	if region != "" {
		awsConfig = map[string]any{
			"region": region,
		}
	}

	return awsConfig
}

// parseAWSScope parses an AWS provider scope and returns the associated region
// Example scope: /planes/aws/aws/accounts/123456789/regions/us-east-1
func parseAWSScope(scope string) (string, error) {
	parsedScope, err := resources.Parse(scope)
	if err != nil {
		return "", v1.NewClientErrInvalidRequest(fmt.Sprintf("Invalid AWS provider scope %q is configured on the Environment, error parsing: %s", scope, err.Error()))
	}

	return parsedScope.FindScope(resources.RegionsSegment), nil
}

func fetchAWSCredentials(ucpConn *sdk.Connection, secretProviderOptions ucp_provider.SecretProviderOptions) (*credentials.AWSCredential, error) {
	awsCredentialProvider, err := credentials.NewAWSCredentialProvider(ucp_provider.NewSecretProvider(secretProviderOptions), *ucpConn, &tokencredentials.AnonymousCredential{})
	if err != nil {
		return nil, fmt.Errorf("error creating AWS credential provider: %w", err)
	}

	credentials, err := awsCredentialProvider.Fetch(context.Background(), credentials.AWSPublic, "default")
	if err != nil {
		return nil, fmt.Errorf("error fetching AWS credentials: %w", err)
	}

	if credentials.AccessKeyID == "" || credentials.SecretAccessKey == "" {
		return nil, errors.New("credentials are required to create AWS resources through Recipe. Use `rad credential register aws` to register AWS credentials")
	}

	return credentials, nil
}
