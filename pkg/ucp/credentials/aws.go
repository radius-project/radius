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

package credentials

import (
	"context"
	"errors"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"

	"github.com/project-radius/radius/pkg/sdk"
	"github.com/project-radius/radius/pkg/to"
	ucpapi "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/secret"
	"github.com/project-radius/radius/pkg/ucp/secret/provider"
)

var _ CredentialProvider[AWSCredential] = (*AWSCredentialProvider)(nil)

// AWSCredentialProvider is UCP credential provider for Azure.
type AWSCredentialProvider struct {
	secretProvider *provider.SecretProvider
	client         *ucpapi.AwsCredentialClient
}

// # Function Explanation
//
// NewAWSCredentialProvider creates a new AWSCredentialProvider struct using the given SecretProvider, UCP connection and
// TokenCredential, and returns it or an error if one occurs.
func NewAWSCredentialProvider(provider *provider.SecretProvider, ucpConn sdk.Connection, credential azcore.TokenCredential) (*AWSCredentialProvider, error) {
	cli, err := ucpapi.NewAwsCredentialClient(credential, sdk.NewClientOptions(ucpConn))
	if err != nil {
		return nil, err
	}

	return &AWSCredentialProvider{
		secretProvider: provider,
		client:         cli,
	}, nil
}

// # Function Explanation
//
// Fetch fetches the AWS IAM access keys from UCP and then from an internal storage (e.g.
// Kubernetes secret store). It returns an AWSCredential struct or an error if the fetch fails.
func (p *AWSCredentialProvider) Fetch(ctx context.Context, planeName, name string) (*AWSCredential, error) {
	// 1. Fetch the secret name of AWS IAM access keys from UCP.
	cred, err := p.client.Get(ctx, planeName, name, &ucpapi.AwsCredentialClientGetOptions{})
	if err != nil {
		return nil, err
	}

	// We support only kubernetes secret, but we may support multiple secret stores.
	var storage *ucpapi.InternalCredentialStorageProperties

	switch p := cred.Properties.(type) {
	case *ucpapi.AWSAccessKeyCredentialProperties:
		switch c := p.Storage.(type) {
		case *ucpapi.InternalCredentialStorageProperties:
			storage = c
		default:
			return nil, errors.New("invalid AWSAccessKeyCredentialProperties")
		}
	default:
		return nil, errors.New("invalid InternalCredentialStorageProperties")
	}

	secretName := to.String(storage.SecretName)
	if secretName == "" {
		return nil, errors.New("unspecified SecretName for internal storage")
	}

	// 2. Fetch the credential from internal storage (e.g. Kubernetes secret store)
	secretClient, err := p.secretProvider.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	s, err := secret.GetSecret[AWSCredential](ctx, secretClient, secretName)
	if err != nil {
		return nil, errors.New("failed to get credential info: " + err.Error())
	}

	return &s, nil
}
