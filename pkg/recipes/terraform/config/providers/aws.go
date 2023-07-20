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
func (p *awsProvider) BuildConfig(ctx context.Context, envConfig *recipes.Configuration) map[string]any {
	logger := ucplog.FromContextOrDiscard(ctx)
	if (envConfig == nil) || (envConfig.Providers == datamodel.Providers{}) || (envConfig.Providers.AWS == datamodel.ProvidersAWS{}) || envConfig.Providers.AWS.Scope == "" {
		logger.Info("AWS provider scope is not configured on the Environment, skipping AWS region configuration.")
		return nil
	}

	region, _ := p.parseScope(ctx, envConfig.Providers.AWS.Scope)
	credentials := fetchAWSCredentials(ctx, p.getCredentialsProvider(ctx))

	return p.generateProviderConfigMap(ctx, credentials, region)
}

func (p *awsProvider) generateProviderConfigMap(ctx context.Context, credentials *credentials.AWSCredential, region string) map[string]any {
	logger := ucplog.FromContextOrDiscard(ctx)
	config := make(map[string]any)

	if region != "" {
		config[AWSRegionParam] = region
	}

	if credentials != nil && credentials.AccessKeyID != "" && credentials.SecretAccessKey != "" {
		config[AWSAccessKeyParam] = credentials.AccessKeyID
		config[AWSSecretKeyParam] = credentials.SecretAccessKey
	} else {
		logger.Info("AWS credentials provider is not configured on the Environment, skipping credentials configuration.")
	}

	return config
}

func (p *awsProvider) parseScope(ctx context.Context, scope string) (region string, err error) {
	// logger := ucplog.FromContextOrDiscard(ctx)
	parsedScope, err := resources.Parse(scope)
	if err != nil {
		return "", v1.NewClientErrInvalidRequest(fmt.Sprintf("Invalid AWS provider scope %q is configured on the Environment, error parsing: %s", scope, err.Error()))
	}

	return parsedScope.FindScope(resources.RegionsSegment), nil
}

func (p *awsProvider) getCredentialsProvider(ctx context.Context) *credentials.AWSCredentialProvider {
	logger := ucplog.FromContextOrDiscard(ctx)
	awsCredentialProvider, err := credentials.NewAWSCredentialProvider(ucp_provider.NewSecretProvider(p.secretProviderOptions), p.ucpConn, &tokencredentials.AnonymousCredential{})
	if err != nil {
		logger.Info(fmt.Sprintf("Error creating AWS credential provider, skipping credentials configuration. Err: %s ", err.Error()))
		return nil
	}

	return awsCredentialProvider
}

// fetchAWSCredentials Fetches AWS credentials from UCP. Returns nil if an error is received or the credentials are empty.
func fetchAWSCredentials(ctx context.Context, awsCredentialsProvider credentials.CredentialProvider[credentials.AWSCredential]) *credentials.AWSCredential {
	logger := ucplog.FromContextOrDiscard(ctx)
	credentials, err := awsCredentialsProvider.Fetch(context.Background(), credentials.AWSPublic, "default")
	if err != nil {
		logger.Info(fmt.Sprintf("Error fetching AWS credentials, skipping credentials configuration. Err: %s", err.Error()))
		return nil
	}

	return credentials
}
