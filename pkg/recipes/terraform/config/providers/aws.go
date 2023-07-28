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
	"github.com/project-radius/radius/pkg/ucp/secret"
	ucp_provider "github.com/project-radius/radius/pkg/ucp/secret/provider"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

// Provider's config parameters need to match the values expected by Terraform
// https://registry.terraform.io/providers/hashicorp/aws/latest/docs
const (
	AWSProviderName   = "aws"
	AWSRegionParam    = "region"
	AWSAccessKeyParam = "access_key"
	AWSSecretKeyParam = "secret_key"
)

type awsProvider struct {
	ucpConn               sdk.Connection
	secretProviderOptions ucp_provider.SecretProviderOptions
}

// NewAWSProvider creates a new AWSProvider instance.
func NewAWSProvider(ucpConn sdk.Connection, secretProviderOptions ucp_provider.SecretProviderOptions) Provider {
	return &awsProvider{ucpConn: ucpConn, secretProviderOptions: secretProviderOptions}
}

// BuildConfig generates the Terraform provider configuration for AWS provider.
// https://registry.terraform.io/providers/hashicorp/aws/latest/docs
func (p *awsProvider) BuildConfig(ctx context.Context, envConfig *recipes.Configuration) (map[string]any, error) {
	region, err := p.parseScope(ctx, envConfig)
	if err != nil {
		return nil, err
	}

	credentialsProvider, err := p.getCredentialsProvider()
	if err != nil {
		return nil, err
	}
	credentials, err := fetchAWSCredentials(ctx, credentialsProvider)
	if err != nil {
		return nil, err
	}

	return p.generateProviderConfigMap(credentials, region), nil
}

// parseScope parses an AWS provider scope and returns the associated region
// Example scope: /planes/aws/aws/accounts/123456789/regions/us-east-1
func (p *awsProvider) parseScope(ctx context.Context, envConfig *recipes.Configuration) (string, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	if (envConfig == nil) || (envConfig.Providers == datamodel.Providers{}) || (envConfig.Providers.AWS == datamodel.ProvidersAWS{}) || envConfig.Providers.AWS.Scope == "" {
		logger.Info("AWS provider/scope is not configured on the Environment, skipping AWS region configuration.")
		return "", nil
	}

	scope := envConfig.Providers.AWS.Scope
	parsedScope, err := resources.Parse(scope)
	if err != nil {
		return "", v1.NewClientErrInvalidRequest(fmt.Sprintf("Invalid AWS provider scope %q is configured on the Environment, error parsing: %s", scope, err.Error()))
	}

	region := parsedScope.FindScope(resources.RegionsSegment)
	if region == "" {
		return "", v1.NewClientErrInvalidRequest(fmt.Sprintf("Invalid AWS provider scope %q is configured on the Environment, region is required in the scope", scope))
	}

	return region, nil
}

func (p *awsProvider) getCredentialsProvider() (*credentials.AWSCredentialProvider, error) {
	awsCredentialProvider, err := credentials.NewAWSCredentialProvider(ucp_provider.NewSecretProvider(p.secretProviderOptions), p.ucpConn, &tokencredentials.AnonymousCredential{})
	if err != nil {
		return nil, err
	}

	return awsCredentialProvider, nil
}

// fetchAWSCredentials Fetches AWS credentials from UCP. Returns nil if credentials not found error is received or the credentials are empty.
func fetchAWSCredentials(ctx context.Context, awsCredentialsProvider credentials.CredentialProvider[credentials.AWSCredential]) (*credentials.AWSCredential, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	credentials, err := awsCredentialsProvider.Fetch(context.Background(), credentials.AWSPublic, "default")
	if err != nil {
		notFound := &secret.ErrNotFound{}
		if notFound.Is(err) {
			logger.Info("AWS credentials are not registered to the Environment, skipping credentials configuration.")
			return nil, nil
		}

		return nil, err
	}

	if credentials == nil || credentials.AccessKeyID == "" || credentials.SecretAccessKey == "" {
		logger.Info("AWS credentials are not registered to the Environment, skipping credentials configuration.")
		return nil, nil
	}

	return credentials, nil
}

func (p *awsProvider) generateProviderConfigMap(credentials *credentials.AWSCredential, region string) map[string]any {
	config := make(map[string]any)
	if region != "" {
		config[AWSRegionParam] = region
	}

	if credentials != nil && credentials.AccessKeyID != "" && credentials.SecretAccessKey != "" {
		config[AWSAccessKeyParam] = credentials.AccessKeyID
		config[AWSSecretKeyParam] = credentials.SecretAccessKey
	}

	return config
}
