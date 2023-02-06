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

//go:generate mockgen -destination=./mock_credentialmanagementclient.go -package=credential -self_package github.com/project-radius/radius/pkg/cli/credential github.com/project-radius/radius/pkg/cli/credential CredentialManagementClient

// CredentialManagementClient is used to interface with cloud provider configuration and credentials.
type CredentialManagementClient interface {
	// Get gets the credential registered with the given ucp provider plane.
	Get(ctx context.Context, providerName string) (ProviderCredentialConfiguration, error)
	// List lists the credentials registered with all ucp provider planes.
	List(ctx context.Context) ([]CloudProviderStatus, error)
	// Put registers a credential with the respective ucp provider plane.
	Put(ctx context.Context, credential_config ucp.CredentialResource) error
	// Delete unregisters credential from the given ucp provider plane.
	Delete(ctx context.Context, providerName string) (bool, error)
}

const (
	AzureCredential      = "azure"
	AWSCredential        = "aws"
	AzurePlaneName       = "azurecloud"
	AWSPlaneName         = "aws"
	azureCredentialKind  = "azure.com.serviceprincipal"
	awsCredentialKind    = "aws.com.iam"
	ValidInfoTemplate    = "enter valid info for %s"
	infoRequiredTemplate = "required info %s"
	defaultSecretName = "default"
)

// UCPCredentialManagementClient implements operations to manage credentials on ucp.
type UCPCredentialManagementClient struct {
	CredentialInterface Interface
}

var _ CredentialManagementClient = (*UCPCredentialManagementClient)(nil)

// Put registers credentials with the provided credential config
func (cpm *UCPCredentialManagementClient) Put(ctx context.Context, credential ucp.CredentialResource) error {
	if strings.EqualFold(*credential.Type, AzureCredential) {
		err := cpm.CredentialInterface.CreateCredential(ctx, AzurePlaneType, AzurePlaneName, defaultSecretName, credential)
		return err
	} else if strings.EqualFold(*credential.Type, AWSCredential) {
		err := cpm.CredentialInterface.CreateCredential(ctx, AWSPlaneType, AWSPlaneName, defaultSecretName, credential)
		return err
	}
	return &ErrUnsupportedCloudProvider{}
}

// Get, gets the credential from the provided ucp provider plane
// We've a single credential configured today for all providers which we name as "default"
// example: If we ask for azure credential, then we will fetch the credential with the name "default" because that is the only
// credential for azure expected in the system.
func (cpm *UCPCredentialManagementClient) Get(ctx context.Context, providerName string) (ProviderCredentialConfiguration, error) {
	var err error
	var cred ProviderCredentialConfiguration
	if strings.EqualFold(providerName, AzureCredential) {
		// We send only the name when getting credentials from backend which we already have access to
		cred, err = cpm.CredentialInterface.GetCredential(ctx, AzurePlaneType, AzurePlaneName, defaultSecretName)
	} else if strings.EqualFold(providerName, AWSCredential) {
		// We send only the name when getting credentials from backend which we already have access to
		cred, err = cpm.CredentialInterface.GetCredential(ctx, AWSPlaneType, AWSPlaneName, defaultSecretName)
	} else {
		return ProviderCredentialConfiguration{}, &ErrUnsupportedCloudProvider{}
	}
	// We get 404 when credential for the provider plane is not registered.
	if clients.Is404Error(err) {
		return ProviderCredentialConfiguration{
			CloudProviderStatus: CloudProviderStatus{
				Name:    providerName,
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
	res, err := cpm.CredentialInterface.ListCredential(ctx, AzurePlaneType, AzurePlaneName)
	if err != nil {
		return nil, err
	}

	// list aws credential
	awsList, err := cpm.CredentialInterface.ListCredential(ctx, AWSPlaneType, AWSPlaneName)
	if err != nil {
		return nil, err
	}
	res = append(res, awsList...)
	return res, nil
}

// Delete, deletes the credentials from the given ucp provider plane
// We've a single credential configured today for all providers which we name as "default"
// example: If we ask to delete azure credential, then we will delete the credential with the name "default" because that is the only
// credential for azure expected in the system.
func (cpm *UCPCredentialManagementClient) Delete(ctx context.Context, providerName string) (bool, error) {
	var err error
	if strings.EqualFold(providerName, AzureCredential) {
		err = cpm.CredentialInterface.DeleteCredential(ctx, AzurePlaneType, AzurePlaneName, defaultSecretName)
	} else if strings.EqualFold(providerName, AWSCredential) {
		err = cpm.CredentialInterface.DeleteCredential(ctx, AWSPlaneType, AWSPlaneName, defaultSecretName)
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
