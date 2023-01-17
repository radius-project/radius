// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ucp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli/clients"
	cli_credential "github.com/project-radius/radius/pkg/cli/credential"
	"github.com/project-radius/radius/pkg/cli/prompt"
	ucp "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
)

const (
	azureCredential      = "azure"
	awsCredential        = "aws"
	azurePlaneName       = "azurecloud"
	awsPlaneName         = "awscloud"
	azureCredentialKind  = "azure.com.serviceprincipal"
	awsCredentialKind    = "aws.com.iam"
	azureCredentialID    = "/planes/azure/azurecloud/providers/System.Azure/credentials/%s"
	awsCredentialID      = "/planes/aws/awscloud/providers/System.AWS/credentials/%s"
	validInfoTemplate    = "enter valid info for %s"
	infoRequiredTemplate = "required info %s"
)

// UCPCredentialManagementClient implements operations to manage credentials on ucp.
type UCPCredentialManagementClient struct {
	CredentialInterface cli_credential.Interface
}

var _ clients.CredentialManagementClient = (*UCPCredentialManagementClient)(nil)

// Put, creates a new cloud provider within ucp azure plane
func (cpm *UCPCredentialManagementClient) Put(ctx context.Context, providerConfig cli_credential.ProviderCredentialConfiguration) error {
	err := validateProviderConfig(providerConfig)
	if err != nil {
		return err
	}
	credential := ucp.CredentialResource{
		Name:     to.Ptr(providerConfig.Name),
		Location: to.Ptr(v1.LocationGlobal),
	}
	if strings.EqualFold(providerConfig.Name, azureCredential) {
		credential.Type = to.Ptr(azureCredential)
		credential.ID = to.Ptr(fmt.Sprintf(azureCredentialID, providerConfig.Name))
		credential.Properties = &ucp.AzureServicePrincipalProperties{
			Storage: &ucp.CredentialStorageProperties{
				Kind: to.Ptr(ucp.CredentialStorageKindInternal),
			},
			TenantID: &providerConfig.AzureCredentials.TenantID,
			ClientID: &providerConfig.AzureCredentials.ClientID,
			Secret:   &providerConfig.AzureCredentials.ClientSecret,
		}
		err := cpm.CredentialInterface.CreateCredential(ctx, cli_credential.AzurePlaneType, azurePlaneName, providerConfig.Name, credential)
		return err
	} else if strings.EqualFold(providerConfig.Name, awsCredential) {
		credential.Type = to.Ptr(awsCredential)
		credential.ID = to.Ptr(fmt.Sprintf(awsCredentialID, providerConfig.Name))
		credential.Properties = &ucp.AWSCredentialProperties{
			Kind: to.Ptr(awsCredentialKind),
			Storage: &ucp.CredentialStorageProperties{
				Kind: to.Ptr(ucp.CredentialStorageKindInternal),
			},
			AccessKeyID:     &providerConfig.AWSCredentials.AccessKeyID,
			SecretAccessKey: &providerConfig.AWSCredentials.SecretAccessKey,
		}
		err := cpm.CredentialInterface.CreateCredential(ctx, cli_credential.AWSPlaneType, awsPlaneName, providerConfig.Name, credential)
		return err
	}
	return &cli_credential.ErrUnsupportedCloudProvider{}
}

// Get, gets the cloud provider with the provided name from ucp azure plane
// TODO: get information except secret data from backend and surface it in this response
func (cpm *UCPCredentialManagementClient) Get(ctx context.Context, name string) (cli_credential.ProviderCredentialResource, error) {
	var err error
	if strings.EqualFold(name, azureCredential) {
		// We send only the name when getting credentials from backend which we already have access to
		err = cpm.CredentialInterface.GetCredential(ctx, cli_credential.AzurePlaneType, azurePlaneName, name)
	} else if strings.EqualFold(name, awsCredential) {
		// We send only the name when getting credentials from backend which we already have access to
		err = cpm.CredentialInterface.GetCredential(ctx, cli_credential.AWSPlaneType, awsPlaneName, name)
	} else {
		return cli_credential.ProviderCredentialResource{}, &cli_credential.ErrUnsupportedCloudProvider{}
	}
	if err != nil {
		// return not enabled if 404
		if clients.Is404Error(err) {
			return cli_credential.ProviderCredentialResource{
				Name:    name,
				Enabled: false,
			}, err
		}
		return cli_credential.ProviderCredentialResource{}, err
	}
	return cli_credential.ProviderCredentialResource{
		Name:    name,
		Enabled: true,
	}, nil
}

// List, lists the cloud providers within ucp azure plane
func (cpm *UCPCredentialManagementClient) List(ctx context.Context) ([]cli_credential.ProviderCredentialResource, error) {
	// list azure credential
	res, err := cpm.CredentialInterface.ListCredential(ctx, cli_credential.AzurePlaneType, azurePlaneName)
	if err != nil {
		return nil, err
	}

	// list aws credential
	awsList, err := cpm.CredentialInterface.ListCredential(ctx, cli_credential.AWSPlaneType, awsPlaneName)
	if err != nil {
		return nil, err
	}
	res = append(res, awsList...)
	return res, nil
}

// Delete, deletes the cloud provider with the name provided in ucp azure plane.
func (cpm *UCPCredentialManagementClient) Delete(ctx context.Context, name string) (bool, error) {
	var err error
	if strings.EqualFold(name, azureCredential) {
		err = cpm.CredentialInterface.DeleteCredential(ctx, cli_credential.AzurePlaneType, azurePlaneName, name)
	} else if strings.EqualFold(name, awsCredential) {
		err = cpm.CredentialInterface.DeleteCredential(ctx, cli_credential.AWSPlaneType, awsPlaneName, name)
	}
	if errors.Is(&cli_credential.ErrUnsupportedCloudProvider{}, err) {
		return false, err
	}
	if err != nil {
		if clients.Is404Error(err) {
			// return true if not found.
			return true, nil
		}
		return false, nil
	}
	return true, nil
}

// func (cpm *UCPProviderCredentialManagementClient) deleteCredentialConfig(ctx context.Context, credentialClient any, planeType string, planeName string, name string) (bool, error) {
// 	var err error
// 	switch credentialClient := credentialClient.(type) {
// 	case *ucp.AzureCredentialClient:
// 		_, err = credentialClient.Delete(ctx, planeType, planeName, name, nil)
// 	case *ucp.AWSCredentialClient:
// 		// We care about success or failure of delete.
// 		_, err = credentialClient.Get(ctx, planeType, planeName, name, nil)
// 	default:
// 		return false, errUnsupportedCloudProvider
// 	}
// 	if err != nil {
// 		if azclient.Is404Error(err) {
// 			// return true if not found.
// 			return true, nil
// 		}
// 		return false, nil
// 	}
// 	return true, nil
// }

func validateProviderConfig(config cli_credential.ProviderCredentialConfiguration) error {
	if config.Name == "" {
		return fmt.Errorf(infoRequiredTemplate, "provider name")
	}
	if config.AzureCredentials != nil {
		isValid, _, _ := prompt.UUIDv4Validator(config.AzureCredentials.ClientID)
		if !isValid {
			return fmt.Errorf(fmt.Sprintf(validInfoTemplate, "azure client id"))
		}
		isValid, _, _ = prompt.UUIDv4Validator(config.AzureCredentials.TenantID)
		if !isValid {
			return fmt.Errorf(validInfoTemplate, "azure tenant id")
		}
		if config.AzureCredentials.ClientSecret == "" {
			return fmt.Errorf(infoRequiredTemplate, "azure client secret")
		}
	}
	if config.AWSCredentials != nil {
		if config.AWSCredentials.AccessKeyID == "" {
			return fmt.Errorf(infoRequiredTemplate, "aws access key")
		}
		if config.AWSCredentials.SecretAccessKey == "" {
			return fmt.Errorf(infoRequiredTemplate, "aws secret access key")
		}
	}
	return nil
}
