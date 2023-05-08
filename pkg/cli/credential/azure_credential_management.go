/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package credential

import (
	"context"
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients"
	ucp "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
)

//go:generate mockgen -destination=./mock_azure_credential_management.go -package=credential -self_package github.com/project-radius/radius/pkg/cli/credential github.com/project-radius/radius/pkg/cli/credential AzureCredentialManagementClientInterface

// AzureCredentialManagementClient is used to interface with cloud provider configuration and credentials.
type AzureCredentialManagementClient struct {
	AzureCredentialClient ucp.AzureCredentialClient
}

// AzureCredentialManagementClient is used to interface with cloud provider configuration and credentials.
type AzureCredentialManagementClientInterface interface {
	// Get gets the credential registered with the given ucp provider plane.
	Get(ctx context.Context, name string) (ProviderCredentialConfiguration, error)
	// List lists the credentials registered with all ucp provider planes.
	List(ctx context.Context) ([]CloudProviderStatus, error)
	// Put registers an AWS credential with the respective ucp provider plane.
	Put(ctx context.Context, credential_config ucp.AzureCredentialResource) error
	// Delete unregisters credential from the given ucp provider plane.
	Delete(ctx context.Context, name string) (bool, error)
}

const (
	AzureCredential     = "azure"
	AzurePlaneName      = "azurecloud"
	azureCredentialKind = "ServicePrincipal"
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
	AzureCredentials *ucp.AzureCredentialProperties

	// AWSCredentials is used to set the credentials on Puts. It is NOT returned on Get/List.
	AWSCredentials *ucp.AWSCredentialProperties
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

// Put registers credentials with the provided credential config
func (cpm *AzureCredentialManagementClient) Put(ctx context.Context, credential ucp.AzureCredentialResource) error {
	if strings.EqualFold(*credential.Type, AzureCredential) {
		_, err := cpm.AzureCredentialClient.CreateOrUpdate(ctx, AzurePlaneName, defaultSecretName, credential, nil)
		return err
	}

	return &ErrUnsupportedCloudProvider{}
}

// Get, gets the credential from the provided ucp provider plane
func (cpm *AzureCredentialManagementClient) Get(ctx context.Context, credentialName string) (ProviderCredentialConfiguration, error) {
	var err error

	resp, err := cpm.AzureCredentialClient.Get(ctx, AzurePlaneName, credentialName, nil)
	if err != nil {
		return ProviderCredentialConfiguration{}, err
	}

	azureServicePrincipal, ok := resp.AzureCredentialResource.Properties.(*ucp.AzureCredentialProperties)
	if !ok {
		return ProviderCredentialConfiguration{}, &cli.FriendlyError{Message: fmt.Sprintf("Unable to Find Credentials for %s", credentialName)}
	}

	providerCredentialConfiguration := ProviderCredentialConfiguration{
		CloudProviderStatus: CloudProviderStatus{
			Name:    AzureCredential,
			Enabled: true,
		},
		AzureCredentials: azureServicePrincipal,
	}

	return providerCredentialConfiguration, nil
}

// List, lists the credentials registered with all ucp provider planes
func (cpm *AzureCredentialManagementClient) List(ctx context.Context) ([]CloudProviderStatus, error) {
	// list azure credential
	var providerList []*ucp.AzureCredentialResource

	pager := cpm.AzureCredentialClient.NewListByRootScopePager(AzurePlaneName, nil)
	for pager.More() {
		nextPage, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		credList := nextPage.AzureCredentialResourceListResult.Value
		providerList = append(providerList, credList...)
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

// Delete, deletes the credentials from the given ucp provider plane
func (cpm *AzureCredentialManagementClient) Delete(ctx context.Context, name string) (bool, error) {
	_, err := cpm.AzureCredentialClient.Delete(ctx, AzurePlaneName, name, nil)

	// We get 404 when credential for the provider plane is not registered.
	if clients.Is404Error(err) {
		// return true if not found.
		return true, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}
