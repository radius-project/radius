// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package credential

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	ucp "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
)

const (
	AzurePlaneType = "azure"
	AWSPlaneType   = "aws"
)

// CloudProviderStatus is the representation of a cloud provider configuration.
type CloudProviderStatus struct {

	// Name is the name/kind of the provider. For right now this only supports Azure.
	Name string

	// Enabled is the enabled/disabled status of the provider.
	Enabled bool
}

type ProviderCredentialConfiguration struct {
	CloudProviderStatus

	// AzureCredentials is used to set the credentials on Puts. It is NOT returned on Get/List.
	AzureCredentials *ucp.AzureServicePrincipalProperties

	// AWSCredentials is used to set the credentials on Puts. It is NOT returned on Get/List.
	AWSCredentials *ucp.AWSCredentialProperties
}

//go:generate mockgen -destination=./mock_client.go -package=credential -self_package github.com/project-radius/radius/pkg/cli/credential github.com/project-radius/radius/pkg/cli/credential Interface
type AWSInterface interface {
	// CreateCredential creates ucp crendential for the supported providers.
	CreateCredential(ctx context.Context, planeType string, planeName string, name string, credential ucp.CredentialResource) error
	// GetCredential gets ucp credentials for the given name if provider is supported.
	GetCredential(ctx context.Context, planeType string, planeName string, name string) (ProviderCredentialConfiguration, error)
	// ListCredential lists ucp credentials configured at the plane scope.
	ListCredential(ctx context.Context, planeType string, planeName string) ([]CloudProviderStatus, error)
	// DeleteCredential deletes ucp credential of the given name if present.
	DeleteCredential(ctx context.Context, planeType string, planeName string, name string) error
}

var _ Interface = (*Impl)(nil)

type Impl struct {
	AzureCredentialClient ucp.AzureCredentialClient
	AWSCredentialClient   ucp.AwsCredentialClient
}

// CreateCredential creates ucp crendential for the supported providers.
func (impl *Impl) CreateCredential(ctx context.Context, planeType string, planeName string, name string, credential ucp.CredentialResource) error {
	switch planeType {
	case AzurePlaneType:
		// We care about success or failure of creation
		_, err := impl.AzureCredentialClient.CreateOrUpdate(ctx, planeType, planeName, name, credential, nil)
		return err
	case AWSPlaneType:
		// We care about success or failure of creation
		_, err := impl.AWSCredentialClient.CreateOrUpdate(ctx, planeType, planeName, name, credential, nil)
		return err
	default:
		return &ErrUnsupportedCloudProvider{}
	}
}

// GetCredential gets ucp credentials for the given name if provider is supported.
func (impl *Impl) GetCredential(ctx context.Context, planeType string, planeName string, name string) (ProviderCredentialConfiguration, error) {
	providerCredentialConfiguration := ProviderCredentialConfiguration{
		CloudProviderStatus: CloudProviderStatus{
			Name:    name,
			Enabled: true,
		},
	}
	switch planeType {
	case AzurePlaneType:
		// We send only the name when getting credentials from backend which we already have access to
		resp, err := impl.AzureCredentialClient.Get(ctx, planeType, planeName, name, nil)
		if err != nil {
			return ProviderCredentialConfiguration{}, err
		}
		azureSPN, ok := resp.CredentialResource.Properties.(*ucp.AzureServicePrincipalProperties)
		if !ok {
			return ProviderCredentialConfiguration{}, &cli.FriendlyError{Message: fmt.Sprintf("Unable to Find Credentials for %s", name)}
		}
		providerCredentialConfiguration.AzureCredentials = azureSPN
		return providerCredentialConfiguration, nil
	case AWSPlaneType:
		// We send only the name when getting credentials from backend which we already have access to
		resp, err := impl.AWSCredentialClient.Get(ctx, planeType, planeName, name, nil)
		if err != nil {
			return ProviderCredentialConfiguration{}, err
		}
		awsIAM, ok := resp.CredentialResource.Properties.(*ucp.AWSCredentialProperties)
		if !ok {
			return ProviderCredentialConfiguration{}, &cli.FriendlyError{Message: fmt.Sprintf("Unable to Find Credentials for %s", name)}
		}
		providerCredentialConfiguration.AWSCredentials = awsIAM
		return providerCredentialConfiguration, err
	default:
		return ProviderCredentialConfiguration{}, &ErrUnsupportedCloudProvider{}
	}
}

// ListCredential lists ucp credentials configured at the plane scope.
func (impl *Impl) ListCredential(ctx context.Context, planeType string, planeName string) ([]CloudProviderStatus, error) {
	var providerList []*ucp.CredentialResource
	switch planeType {
	case AzurePlaneType:
		resp, err := impl.AzureCredentialClient.List(ctx, planeType, planeName, nil)
		if err != nil {
			return nil, err
		}
		providerList = resp.CredentialResourceList.Value
	case AWSPlaneType:
		resp, err := impl.AWSCredentialClient.List(ctx, planeType, planeName, nil)
		if err != nil {
			return nil, err
		}
		providerList = resp.CredentialResourceList.Value
	default:
		return nil, &ErrUnsupportedCloudProvider{}
	}
	res := make([]CloudProviderStatus, 0)
	for _, provider := range providerList {
		res = append(res, CloudProviderStatus{
			Name:    *provider.Name,
			Enabled: true,
		})
	}
	return res, nil
}

// DeleteCredential deletes ucp credential of the given name if present.
func (impl *Impl) DeleteCredential(ctx context.Context, planeType string, planeName string, name string) error {
	switch planeType {
	case AzurePlaneType:
		// We care about success or failure of delete.
		_, err := impl.AzureCredentialClient.Delete(ctx, planeType, planeName, name, nil)
		return err
	case AWSPlaneType:
		// We care about success or failure of delete.
		_, err := impl.AWSCredentialClient.Delete(ctx, planeType, planeName, name, nil)
		return err
	default:
		return &ErrUnsupportedCloudProvider{}
	}
}

// ErrUnsupportedCloudProvider represents error when the cloud provider is not supported by radius.
type ErrUnsupportedCloudProvider struct {
	Message string
}

func (fe *ErrUnsupportedCloudProvider) Error() string {
	return "unsupported cloud provider"
}

func (fe *ErrUnsupportedCloudProvider) Is(target error) bool {
	_, ok := target.(*ErrUnsupportedCloudProvider)
	return ok
}
