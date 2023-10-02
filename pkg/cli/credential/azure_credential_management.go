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
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	ucp "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
)

//go:generate mockgen -destination=./mock_azure_credential_management.go -package=credential -self_package github.com/radius-project/radius/pkg/cli/credential github.com/radius-project/radius/pkg/cli/credential AzureCredentialManagementClientInterface

// AzureCredentialManagementClient is used to interface with cloud provider configuration and credentials.
type AzureCredentialManagementClient struct {
	AzureCredentialClient ucp.AzureCredentialsClient
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
	// Name is the name/kind of the provider. For right now this only supports Azure and AWS.
	Name string

	// Enabled is the enabled/disabled status of the provider.
	Enabled bool
}

type ProviderCredentialConfiguration struct {
	CloudProviderStatus

	// AzureCredentials is used to set the credentials on Puts. It is NOT returned on Get/List.
	AzureCredentials *AzureCredentialProperties

	// AWSCredentials is used to set the credentials on Puts. It is NOT returned on Get/List.
	AWSCredentials *AWSCredentialProperties
}

type AzureCredentialProperties struct {
	// clientId for ServicePrincipal
	ClientID *string

	// The credential kind
	Kind *string

	// tenantId for ServicePrincipal
	TenantID *string
}

// ErrUnsupportedCloudProvider represents error when the cloud provider is not supported by radius.
type ErrUnsupportedCloudProvider struct {
	Message string
}

// ErrUnsupportedCloudProvider's Error() function returns a string indicating an unsupported cloud provider when called.
func (fe *ErrUnsupportedCloudProvider) Error() string {
	return "unsupported cloud provider"
}

// Is() checks if the target error is of type ErrUnsupportedCloudProvider and returns a boolean value indicating the result.
func (fe *ErrUnsupportedCloudProvider) Is(target error) bool {
	_, ok := target.(*ErrUnsupportedCloudProvider)
	return ok
}

// Put registers credentials with the provided credential config
//

// "Put" checks if the credential type is supported by the AzureCredentialManagementClient, and if so, creates or updates
// the credential in Azure, otherwise it returns an error.
func (cpm *AzureCredentialManagementClient) Put(ctx context.Context, credential ucp.AzureCredentialResource) error {
	if strings.EqualFold(*credential.Type, AzureCredential) {
		_, err := cpm.AzureCredentialClient.CreateOrUpdate(ctx, AzurePlaneName, defaultSecretName, credential, nil)
		return err
	}

	return &ErrUnsupportedCloudProvider{}
}

// Get, gets the credential from the provided ucp provider plane
//

// "Get" retrieves an AzureCredentialResource from the AzureCredentialClient and returns a ProviderCredentialConfiguration
// object, or an error if the retrieval fails.
func (cpm *AzureCredentialManagementClient) Get(ctx context.Context, credentialName string) (ProviderCredentialConfiguration, error) {
	var err error

	resp, err := cpm.AzureCredentialClient.Get(ctx, AzurePlaneName, credentialName, nil)
	if err != nil {
		return ProviderCredentialConfiguration{}, err
	}

	azureServicePrincipal, ok := resp.AzureCredentialResource.Properties.(*ucp.AzureServicePrincipalProperties)
	if !ok {
		return ProviderCredentialConfiguration{}, clierrors.Message("Unable to find credentials for cloud provider %s.", AzureCredential)
	}

	providerCredentialConfiguration := ProviderCredentialConfiguration{
		CloudProviderStatus: CloudProviderStatus{
			Name:    AzureCredential,
			Enabled: true,
		},
		AzureCredentials: &AzureCredentialProperties{
			ClientID: azureServicePrincipal.ClientID,
			Kind:     (*string)(azureServicePrincipal.Kind),
			TenantID: azureServicePrincipal.TenantID,
		},
	}

	return providerCredentialConfiguration, nil
}

// List, lists the credentials registered with all ucp provider planes
//

// List retrieves a list of Azure credentials and returns a slice of CloudProviderStatus
// objects containing the name and enabled status of each credential.
func (cpm *AzureCredentialManagementClient) List(ctx context.Context) ([]CloudProviderStatus, error) {
	// list azure credential
	var providerList []*ucp.AzureCredentialResource

	pager := cpm.AzureCredentialClient.NewListPager(AzurePlaneName, nil)
	for pager.More() {
		nextPage, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		credList := nextPage.AzureCredentialResourceListResult.Value
		providerList = append(providerList, credList...)
	}

	res := []CloudProviderStatus{}
	if len(providerList) > 0 {
		res = append(res, CloudProviderStatus{
			Name:    AzureCredential,
			Enabled: true,
		})
	}

	return res, nil
}

// Delete, deletes the credentials from the given ucp provider plane
//

// "Delete"  checks if the credential for the provider plane is registered and returns true if not found, otherwise
// returns false and an error if one occurs.
func (cpm *AzureCredentialManagementClient) Delete(ctx context.Context, name string) (bool, error) {
	var respFromCtx *http.Response
	ctxWithResp := runtime.WithCaptureResponse(ctx, &respFromCtx)
	_, err := cpm.AzureCredentialClient.Delete(ctxWithResp, AzurePlaneName, name, nil)
	if err != nil {
		return false, err
	}
	return respFromCtx.StatusCode != 204, nil
}
