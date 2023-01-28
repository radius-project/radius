// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package credential

import (
	"context"
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients"
	ucp "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
)

//go:generate mockgen -destination=./mock_credentialmanagementclient.go -package=credential -self_package github.com/project-radius/radius/pkg/cli/credential github.com/project-radius/radius/pkg/cli/credential CredentialManagementClient

// CredentialManagementClient is used to interface with cloud provider configuration and credentials.
type AzureCredentialManagementClient struct {
	AzureCredentialClient ucp.AzureCredentialClient
}

const (
	AzureCredential     = "azure"
	AzurePlaneName      = "azurecloud"
	azureCredentialKind = "ServicePrincipal"
)

var _ CredentialManagementClient = (*UCPCredentialManagementClient)(nil)

// Put registers credentials with the provided credential config
func (cpm *AzureCredentialManagementClient) Put(ctx context.Context, credential ucp.AzureCredentialResource) error {
	if strings.EqualFold(*credential.Type, AzureCredential) {
		_, err := cpm.AzureCredentialClient.CreateOrUpdate(ctx, *credential.Name, credential, nil)
		return err
	}
	return &ErrUnsupportedCloudProvider{}
}

// Get, gets the credential from the provided ucp provider plane
// TODO: get information except secret data from backend and surface it in this response
func (cpm *AzureCredentialManagementClient) Get(ctx context.Context, name string) (ProviderCredentialConfiguration, error) {
	var err error
	providerCredentialConfiguration := ProviderCredentialConfiguration{
		CloudProviderStatus: CloudProviderStatus{
			Name:    name,
			Enabled: true,
		},
	}

	if strings.EqualFold(name, AzureCredential) {
		// We send only the name when getting credentials from backend which we already have access to
		resp, err := cpm.AzureCredentialClient.Get(ctx, name, nil)
		if err != nil {
			return ProviderCredentialConfiguration{}, err
		}
		azureServicePrincipal, ok := resp.AzureCredentialResource.Properties.(*ucp.AzureCredentialProperties)
		if !ok {
			return ProviderCredentialConfiguration{}, &cli.FriendlyError{Message: fmt.Sprintf("Unable to Find Credentials for %s", name)}
		}
		providerCredentialConfiguration.AzureCredentials = azureServicePrincipal
	} else {
		return ProviderCredentialConfiguration{}, &ErrUnsupportedCloudProvider{}
	}

	// We get 404 when credential for the provider plane is not registered.
	if clients.Is404Error(err) {
		return ProviderCredentialConfiguration{
			CloudProviderStatus: CloudProviderStatus{
				Name:    name,
				Enabled: false,
			},
		}, nil
	} else if err != nil {
		return ProviderCredentialConfiguration{}, err
	}
	return providerCredentialConfiguration, nil
}

// List, lists the credentials registered with all ucp provider planes
func (cpm *AzureCredentialManagementClient) List(ctx context.Context) ([]CloudProviderStatus, error) {
	// list azure credential
	var providerList []*ucp.AzureCredentialResource

	pager := cpm.AzureCredentialClient.NewListByRootScopePager(nil)
	for pager.More() {
		nextPage, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		credList := nextPage.AzureCredentialResourceListResult.Value
		for _, resource := range credList {
			providerList = append(providerList, resource)
		}
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
	_, err := cpm.AzureCredentialClient.Delete(ctx, name, nil)
	// We get 404 when credential for the provider plane is not registered.
	if clients.Is404Error(err) {
		// return true if not found.
		return true, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}
