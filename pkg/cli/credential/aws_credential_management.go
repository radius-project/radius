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

//go:generate mockgen -destination=./mock_aws_credential_management.go -package=credential -self_package github.com/project-radius/radius/pkg/cli/credential github.com/project-radius/radius/pkg/cli/credential AWSCredentialManagementClientInterface

// CredentialManagementClient is used to interface with cloud provider configuration and credentials.
type AWSCredentialManagementClient struct {
	AWSCredentialClient ucp.AwsCredentialClient
}

const (
	AWSCredential        = "aws"
	AWSPlaneName         = "aws"
	awsCredentialKind    = "AccessKey"
	ValidInfoTemplate    = "enter valid info for %s"
	infoRequiredTemplate = "required info %s"
)

// CredentialManagementClient is used to interface with cloud provider configuration and credentials.
type AWSCredentialManagementClientInterface interface {
	// Get gets the credential registered with the given ucp provider plane.
	Get(ctx context.Context, name string) (ProviderCredentialConfiguration, error)
	// List lists the credentials registered with all ucp provider planes.
	List(ctx context.Context) ([]CloudProviderStatus, error)
	// Put registers an AWS credential with the respective ucp provider plane.
	Put(ctx context.Context, credential_config ucp.AWSCredentialResource) error
	// Delete unregisters credential from the given ucp provider plane.
	Delete(ctx context.Context, name string) (bool, error)
}

// Put registers credentials with the provided credential config
func (cpm *AWSCredentialManagementClient) Put(ctx context.Context, credential ucp.AWSCredentialResource) error {
	if strings.EqualFold(*credential.Type, AWSCredential) {
		_, err := cpm.AWSCredentialClient.CreateOrUpdate(ctx, AWSPlaneName, defaultSecretName, credential, nil)
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
		resp, err := cpm.AWSCredentialClient.Get(ctx, AWSPlaneName, name, nil)
		if err != nil {
			return ProviderCredentialConfiguration{}, err
		}
		awsAccessKeyCredentials, ok := resp.AWSCredentialResource.Properties.(*ucp.AWSCredentialProperties)
		if !ok {
			return ProviderCredentialConfiguration{}, &cli.FriendlyError{Message: fmt.Sprintf("Unable to Find Credentials for %s", name)}
		}
		providerCredentialConfiguration.AWSCredentials = awsAccessKeyCredentials
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

// List, lists the AWS credentials registered
func (cpm *AWSCredentialManagementClient) List(ctx context.Context) ([]CloudProviderStatus, error) {
	// list AWS credential
	var providerList []*ucp.AWSCredentialResource

	pager := cpm.AWSCredentialClient.NewListByRootScopePager(AWSPlaneName, nil)
	for pager.More() {
		nextPage, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		credList := nextPage.AWSCredentialResourceListResult.Value
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
func (cpm *AWSCredentialManagementClient) Delete(ctx context.Context, name string) (bool, error) {
	_, err := cpm.AWSCredentialClient.Delete(ctx, AWSPlaneName, name, nil)
	// We get 404 when credential for the provider plane is not registered.
	if clients.Is404Error(err) {
		// return true if not found.
		return true, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}
