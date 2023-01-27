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
type AWSCredentialManagementClient struct {
	AWSCredentialClient ucp.AwsCredentialClient
}

const (
	AzureCredential      = "azure"
	AWSCredential        = "aws"
	AzurePlaneName       = "azurecloud"
	AWSPlaneName         = "aws"
	azureCredentialKind  = "ServicePrincipal"
	awsCredentialKind    = "AccessKey"
	ValidInfoTemplate    = "enter valid info for %s"
	infoRequiredTemplate = "required info %s"
)

// // UCPCredentialManagementClient implements operations to manage credentials on ucp.
// type AWSCredentialManagementClient struct {
// 	CredentialInterface Interface
// }

var _ CredentialManagementClient = (*UCPCredentialManagementClient)(nil)

// Put registers credentials with the provided credential config
func (cpm *AWSCredentialManagementClient) Put(ctx context.Context, credential ucp.AWSCredentialResource) error {
	if strings.EqualFold(*credential.Type, AWSCredential) {
		_, err := cpm.AWSCredentialClient.CreateOrUpdate(ctx, *credential.Name, credential, nil)
		return err
	}
	return &ErrUnsupportedCloudProvider{}
}

// Get, gets the credential from the provided ucp provider plane
// TODO: get information except secret data from backend and surface it in this response
func (cpm *AWSCredentialManagementClient) Get(ctx context.Context, name string) (ProviderCredentialConfiguration, error) {
	var err error
	providerCredentialConfiguration := ProviderCredentialConfiguration{
		CloudProviderStatus: CloudProviderStatus{
			Name:    name,
			Enabled: true,
		},
	}

	if strings.EqualFold(name, AWSCredential) {
		// We send only the name when getting credentials from backend which we already have access to
		// cred, err = cpm.CredentialInterface.GetCredential(ctx, AWSPlaneType, AWSPlaneName, name)
		resp, err := cpm.AWSCredentialClient.Get(ctx, name, nil)
		if err != nil {
			return ProviderCredentialConfiguration{}, err
		}
		awsIAM, ok := resp.AWSCredentialResource.Properties.(*ucp.AWSCredentialProperties)
		if !ok {
			return ProviderCredentialConfiguration{}, &cli.FriendlyError{Message: fmt.Sprintf("Unable to Find Credentials for %s", name)}
		}
		providerCredentialConfiguration.AWSCredentials = awsIAM
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
func (cpm *AWSCredentialManagementClient) List(ctx context.Context) ([]CloudProviderStatus, error) {
	// list azure credential
	resp, err := cpm.AWSCredentialClient.NewListByRootScopePager()

	if err != nil {
		return nil, err
	}
	providerList = resp.CredentialResourceList.Value

	res = append(res, awsList...)
	return res, nil
}

// Delete, deletes the credentials from the given ucp provider plane
func (cpm *AWSCredentialManagementClient) Delete(ctx context.Context, name string) (bool, error) {
	var err error
	if strings.EqualFold(name, AzureCredential) {
		err = cpm.CredentialInterface.DeleteCredential(ctx, AzurePlaneType, AzurePlaneName, name)
	} else if strings.EqualFold(name, AWSCredential) {
		err = cpm.CredentialInterface.DeleteCredential(ctx, AWSPlaneType, AWSPlaneName, name)
	}
	// We get 404 when credential for the provider plane is not registered.
	if clients.Is404Error(err) {
		// return true if not found.
		return true, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}
