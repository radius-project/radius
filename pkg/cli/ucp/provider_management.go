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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	azclient "github.com/project-radius/radius/pkg/azure/clients"
	aztoken "github.com/project-radius/radius/pkg/azure/tokencredentials"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/prompt"
	ucp "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
)

const (
	azureCloudProvider   = "Azure"
	awsCloudProvider     = "AWS"
	azurePlaneName       = "azurecloud"
	awsPlaneName         = "awscloud"
	azureCredentialType  = "System.Azure/credentials"
	awsCredentialType    = "System.AWS/credentials"
	azureCredentialKind  = "azure.com.serviceprincipal"
	awsCredentialKind    = "aws.com.iam"
	azureCredentialID    = "/planes/azure/azurecloud/providers/System.Azure/credentials/%s"
	awsCredentialID      = "/planes/aws/awscloud/providers/System.AWS/credentials/%s"
	infoRequiredTemplate = "required Info %s"
)

var errUnsupportedCloudProvider = errors.New("unsupported Cloud Provider")

type UCPCloudProviderManagementClient struct {
	ClientOptions *arm.ClientOptions
}

var _ clients.CloudProviderManagementClient = (*UCPCloudProviderManagementClient)(nil)

// Put, creates a new cloud provider within ucp azure plane
func (cpm *UCPCloudProviderManagementClient) Put(ctx context.Context, providerConfig clients.CloudProviderConfiguration) error {
	err := validateProviderConfig(providerConfig)
	if err != nil {
		return err
	}
	if strings.EqualFold(providerConfig.Name, azureCloudProvider) {
		credentialClient, err := ucp.NewAzureCredentialClient(&aztoken.AnonymousCredential{}, cpm.ClientOptions)
		if err != nil {
			return err
		}
		return cpm.createCredentialConfiguration(ctx, credentialClient, providerConfig.Name, providerConfig)
	} else if strings.EqualFold(providerConfig.Name, awsCloudProvider) {
		credentialClient, err := ucp.NewAzureCredentialClient(&aztoken.AnonymousCredential{}, cpm.ClientOptions)
		if err != nil {
			return err
		}
		return cpm.createCredentialConfiguration(ctx, credentialClient, providerConfig.Name, providerConfig)
	}
	return errUnsupportedCloudProvider
}

func (cpm *UCPCloudProviderManagementClient) createCredentialConfiguration(ctx context.Context, credentialClient any, name string, providerConfig clients.CloudProviderConfiguration) error {
	credential := ucp.CredentialResource{
		Name:     to.Ptr(name),
		Location: to.Ptr(v1.LocationGlobal),
	}

	switch client := credentialClient.(type) {
	case *ucp.AzureCredentialClient:
		credential.Type = to.Ptr(azureCloudProvider)
		credential.ID = to.Ptr(fmt.Sprintf(azureCredentialID, name))
		credential.Properties = &ucp.AzureServicePrincipalProperties{
			Storage: &ucp.CredentialStorageProperties{
				Kind: to.Ptr(ucp.CredentialStorageKindInternal),
			},
			TenantID: &providerConfig.AzureCredentials.TenantID,
			ClientID: &providerConfig.AzureCredentials.ClientID,
			Secret:   &providerConfig.AzureCredentials.ClientSecret,
		}
		_, err := client.CreateOrUpdate(ctx, strings.ToLower(azureCloudProvider), azurePlaneName, name, credential, nil)
		return err
	case *ucp.AWSCredentialClient:
		credential.Type = to.Ptr(awsCloudProvider)
		credential.ID = to.Ptr(fmt.Sprintf(awsCredentialID, name))
		credential.Properties = &ucp.AWSCredentialProperties{
			Kind: to.Ptr(awsCredentialKind),
			Storage: &ucp.CredentialStorageProperties{
				Kind: to.Ptr(ucp.CredentialStorageKindInternal),
			},
			AccessKeyID:     &providerConfig.AWSCredentials.AccessKeyID,
			SecretAccessKey: &providerConfig.AWSCredentials.SecretAccessKey,
		}
		_, err := client.CreateOrUpdate(ctx, strings.ToLower(awsCloudProvider), awsPlaneName, name, credential, nil)
		return err
	default:
		return errUnsupportedCloudProvider
	}
}

// Get, gets the cloud provider with the provided name from ucp azure plane
func (cpm *UCPCloudProviderManagementClient) Get(ctx context.Context, name string) (clients.CloudProviderResource, error) {
	if strings.EqualFold(name, azureCloudProvider) {
		credentialClient, err := ucp.NewAzureCredentialsClient(&aztoken.AnonymousCredential{}, cpm.ClientOptions)
		if err != nil {
			return clients.CloudProviderResource{}, err
		}
		return cpm.getCredentialConfig(ctx, credentialClient, strings.ToLower(azureCloudProvider), azurePlaneName, name)
	} else if strings.EqualFold(name, awsCloudProvider) {
		credentialClient, err := ucp.NewAWSCredentialsClient(&aztoken.AnonymousCredential{}, cpm.ClientOptions)
		if err != nil {
			return clients.CloudProviderResource{}, err
		}
		return cpm.getCredentialConfig(ctx, credentialClient, strings.ToLower(awsCloudProvider), awsPlaneName, name)
	}
	return clients.CloudProviderResource{}, errUnsupportedCloudProvider
}

func (cpm *UCPCloudProviderManagementClient) getCredentialConfig(ctx context.Context, credentialClient any,
	planeType string, planeName string, name string) (clients.CloudProviderResource, error) {
	var err error
	switch credentialClient := credentialClient.(type) {
	case *ucp.AzureCredentialsClient:
		// We send only the name when getting credentials from backend which we already have access to
		_, err = credentialClient.Get(ctx, planeType, planeName, name, nil)
	case *ucp.AWSCredentialsClient:
		// We send only the name when getting credentials from backend which we already have access to
		_, err = credentialClient.Get(ctx, planeType, planeName, name, nil)
	default:
		return clients.CloudProviderResource{}, errUnsupportedCloudProvider
	}
	if err != nil {
		// return not enabled if 404
		if clients.Is404Error(err) {
			return clients.CloudProviderResource{
				Name:    name,
				Enabled: false,
			}, err
		}
		return clients.CloudProviderResource{}, err
	}
	return clients.CloudProviderResource{
		Name:    name,
		Enabled: true,
	}, nil
}

// List, lists the cloud providers within ucp azure plane
func (cpm *UCPCloudProviderManagementClient) List(ctx context.Context) ([]clients.CloudProviderResource, error) {
	// list azure credential
	azureCredentialClient, err := ucp.NewAzureCredentialsClient(&aztoken.AnonymousCredential{}, cpm.ClientOptions)
	if err != nil {
		return nil, err
	}
	res, err := cpm.listProviderConfigs(ctx, azureCredentialClient)
	if err != nil {
		return nil, err
	}

	// list aws credential
	awsCredentialClient, err := ucp.NewAWSCredentialsClient(&aztoken.AnonymousCredential{}, cpm.ClientOptions)
	if err != nil {
		return nil, err
	}
	awsList, err := cpm.listProviderConfigs(ctx, awsCredentialClient)
	if err != nil {
		return nil, err
	}
	if len(awsList) > 0 {
		res = append(res, awsList...)
	}
	return res, nil
}

func (cpm *UCPCloudProviderManagementClient) listProviderConfigs(ctx context.Context, credentialClient any) ([]clients.CloudProviderResource, error) {
	var providerList []*ucp.CredentialResource
	switch credentialClient := credentialClient.(type) {
	case *ucp.AzureCredentialsClient:
		resp, err := credentialClient.List(ctx, strings.ToLower(azureCloudProvider), azurePlaneName, nil)
		if err != nil {
			return nil, err
		}
		providerList = resp.CredentialResourceList.Value
	case *ucp.AWSCredentialsClient:
		resp, err := credentialClient.List(ctx, strings.ToLower(awsCloudProvider), awsPlaneName, nil)
		if err != nil {
			return nil, err
		}
		providerList = resp.CredentialResourceList.Value
	default:
		return nil, errUnsupportedCloudProvider
	}
	res := make([]clients.CloudProviderResource, 0)
	for _, provider := range providerList {
		res = append(res, clients.CloudProviderResource{
			Name:    *provider.Name,
			Enabled: true,
		})
	}
	return res, nil
}

// Delete, deletes the cloud provider with the name provided in ucp azure plane.
func (cpm *UCPCloudProviderManagementClient) Delete(ctx context.Context, name string) (bool, error) {
	if strings.EqualFold(name, azureCloudProvider) {
		credentialClient, err := ucp.NewAzureCredentialsClient(&aztoken.AnonymousCredential{}, cpm.ClientOptions)
		if err != nil {
			return false, err
		}
		return cpm.deleteCredentialConfig(ctx, credentialClient, strings.ToLower(azureCloudProvider), azurePlaneName, name)
	} else if strings.EqualFold(name, awsCloudProvider) {
		credentialClient, err := ucp.NewAWSCredentialsClient(&aztoken.AnonymousCredential{}, cpm.ClientOptions)
		if err != nil {
			return false, err
		}
		return cpm.deleteCredentialConfig(ctx, credentialClient, strings.ToLower(awsCloudProvider), awsPlaneName, name)
	}
	return false, errUnsupportedCloudProvider
}

func (cpm *UCPCloudProviderManagementClient) deleteCredentialConfig(ctx context.Context, credentialClient any, planeType string, planeName string, name string) (bool, error) {
	var err error
	switch credentialClient := credentialClient.(type) {
	case *ucp.AzureCredentialsClient:
		_, err = credentialClient.Delete(ctx, planeType, planeName, name, nil)
	case *ucp.AWSCredentialsClient:
		// We care about success or failure of delete.
		_, err = credentialClient.Get(ctx, planeType, planeName, name, nil)
	default:
		return false, errUnsupportedCloudProvider
	}
	if err != nil {
		if azclient.Is404Error(err) {
			// return true if not found.
			return true, nil
		}
		return false, nil
	}
	return true, nil
}

func validateProviderConfig(config clients.CloudProviderConfiguration) error {
	if config.Name == "" {
		return fmt.Errorf(infoRequiredTemplate, "provider name")
	}
	if config.AzureCredentials != nil {
		isValid, _, _ := prompt.UUIDv4Validator(config.AzureCredentials.ClientID)
		if !isValid {
			return fmt.Errorf(fmt.Sprintf(infoRequiredTemplate, "azure client id"))
		}
		isValid, _, _ = prompt.UUIDv4Validator(config.AzureCredentials.TenantID)
		if !isValid {
			return fmt.Errorf(infoRequiredTemplate, "azure tenant id")
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
