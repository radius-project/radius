// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package credential

import (
	"context"
	"strings"

	"github.com/project-radius/radius/pkg/cli/clients"
	ucp "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
)

const (
	AzurePlaneType = "azure"
	AWSPlaneType   = "aws"
)

//go:generate mockgen -destination=./mock_credentialmanagementclient.go -package=credential -self_package github.com/project-radius/radius/pkg/cli/credential github.com/project-radius/radius/pkg/cli/credential CredentialManagementClient

// CredentialManagementClient is used to interface with cloud provider configuration and credentials.
type CredentialManagementClient interface {
	// Get gets the credential registered with the given ucp provider plane.
	Get(ctx context.Context, name string) (ProviderCredentialConfiguration, error)
	// List lists the credentials registered with all ucp provider planes.
	List(ctx context.Context) ([]CloudProviderStatus, error)
	// Put registers an AWS credential with the respective ucp provider plane.
	PutAWS(ctx context.Context, credential_config ucp.AWSCredentialResource) error
	// Put registers an AWS credential with the respective ucp provider plane.
	PutAzure(ctx context.Context, credential_config ucp.AzureCredentialResource) error
	// Delete unregisters credential from the given ucp provider plane.
	Delete(ctx context.Context, name string) (bool, error)
}

// UCPCredentialManagementClient implements operations to manage credentials on ucp.
type UCPCredentialManagementClient struct {
	AzClient  AzureCredentialManagementClientInterface
	AWSClient AWSCredentialManagementClientInterface
}

var _ CredentialManagementClient = (*UCPCredentialManagementClient)(nil)

// Put registers credentials with the provided credential config
func (cpm *UCPCredentialManagementClient) PutAWS(ctx context.Context, credential ucp.AWSCredentialResource) error {
	err := cpm.AWSClient.Put(ctx, credential)
	return err
}

// Put registers credentials with the provided credential config
func (cpm *UCPCredentialManagementClient) PutAzure(ctx context.Context, credential ucp.AzureCredentialResource) error {
	err := cpm.AzClient.Put(ctx, credential)
	return err
}

// Get, gets the credential from the provided ucp provider plane
// TODO: get information except secret data from backend and surface it in this response
func (cpm *UCPCredentialManagementClient) Get(ctx context.Context, name string) (ProviderCredentialConfiguration, error) {
	var err error
	var cred ProviderCredentialConfiguration
	if strings.EqualFold(name, AzureCredential) {
		// We send only the name when getting credentials from backend which we already have access to
		cred, err = cpm.AzClient.Get(ctx, name)
	} else if strings.EqualFold(name, AWSCredential) {
		// We send only the name when getting credentials from backend which we already have access to
		cred, err = cpm.AWSClient.Get(ctx, name)
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
	return cred, nil
}

// List, lists the credentials registered with all ucp provider planes
func (cpm *UCPCredentialManagementClient) List(ctx context.Context) ([]CloudProviderStatus, error) {
	// list azure credential
	res, err := cpm.AzClient.List(ctx)
	if err != nil {
		return nil, err
	}

	// list aws credential
	awsList, err := cpm.AWSClient.List(ctx)
	if err != nil {
		return nil, err
	}
	res = append(res, awsList...)
	return res, nil
}

// Delete, deletes the credentials from the given ucp provider plane
func (cpm *UCPCredentialManagementClient) Delete(ctx context.Context, name string) (bool, error) {
	if strings.EqualFold(name, AzureCredential) {
		return cpm.AzClient.Delete(ctx, name)
	} else if strings.EqualFold(name, AWSCredential) {
		return cpm.AWSClient.Delete(ctx, name)
	}
	return true, nil
}
