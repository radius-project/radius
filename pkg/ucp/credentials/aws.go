// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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

// NewAWSCredentialProvider creates new AWSCredentialProvider.
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

// Fetch gets the AWS IAM credentials from secret storage.
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
			return nil, errors.New("invalid InternalCredentialStorageProperties")
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
