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

package credential

import (
	"context"
	"strings"

	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/clierrors"
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

// # Function Explanation
// 
//	ErrUnsupportedCloudProvider is an error function that returns an error message when an unsupported cloud provider is 
//	encountered. It is useful for callers of this function to handle errors related to unsupported cloud providers.
func (fe *ErrUnsupportedCloudProvider) Error() string {
	return "unsupported cloud provider"
}

// # Function Explanation
// 
//	ErrUnsupportedCloudProvider's Is() method checks if the given error is of type ErrUnsupportedCloudProvider, and returns 
//	a boolean value indicating the result of the check. This allows callers of the function to handle the error accordingly.
func (fe *ErrUnsupportedCloudProvider) Is(target error) bool {
	_, ok := target.(*ErrUnsupportedCloudProvider)
	return ok
}

// Put registers credentials with the provided credential config
//
// # Function Explanation
// 
//	The Put function of the AzureCredentialManagementClient creates or updates an Azure credential resource in the Azure 
//	plane. It returns an error if the credential type is not supported or if there is an issue with the request.
func (cpm *AzureCredentialManagementClient) Put(ctx context.Context, credential ucp.AzureCredentialResource) error {
	if strings.EqualFold(*credential.Type, AzureCredential) {
		_, err := cpm.AzureCredentialClient.CreateOrUpdate(ctx, AzurePlaneName, defaultSecretName, credential, nil)
		return err
	}

	return &ErrUnsupportedCloudProvider{}
}

// Get, gets the credential from the provided ucp provider plane
//
// # Function Explanation
// 
//	The Get function of the AzureCredentialManagementClient retrieves the AzureCredentialProperties associated with the 
//	given credentialName from the Azure plane. If the credentialName is not found, an error is returned.
func (cpm *AzureCredentialManagementClient) Get(ctx context.Context, credentialName string) (ProviderCredentialConfiguration, error) {
	var err error

	resp, err := cpm.AzureCredentialClient.Get(ctx, AzurePlaneName, credentialName, nil)
	if err != nil {
		return ProviderCredentialConfiguration{}, err
	}

	azureServicePrincipal, ok := resp.AzureCredentialResource.Properties.(*ucp.AzureCredentialProperties)
	if !ok {
		return ProviderCredentialConfiguration{}, clierrors.Message("Unable to find credentials for cloud provider %s.", credentialName)
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
//
// # Function Explanation
// 
//	The List function of the AzureCredentialManagementClient retrieves a list of all the Azure credentials stored in the 
//	root scope and returns them as a slice of CloudProviderStatus objects. It handles errors by returning them to the 
//	caller.
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

	res := []CloudProviderStatus{}
	for _, provider := range providerList {
		res = append(res, CloudProviderStatus{
			Name:    *provider.Name,
			Enabled: true,
		})
	}

	return res, nil
}

// Delete, deletes the credentials from the given ucp provider plane
//
// # Function Explanation
// 
//	The Delete function of the AzureCredentialManagementClient attempts to delete a credential from the AzurePlaneName and 
//	returns a boolean and an error. If the credential is not found, it returns true and no error. If an error is 
//	encountered, it returns false and the error.
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
