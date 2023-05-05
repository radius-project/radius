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

//go:generate mockgen -destination=./mock_aws_credential_management.go -package=credential -self_package github.com/project-radius/radius/pkg/cli/credential github.com/project-radius/radius/pkg/cli/credential AWSCredentialManagementClientInterface

// AWSCredentialManagementClient is used to interface with cloud provider configuration and credentials.
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

// AWSCredentialManagementClient is used to interface with cloud provider configuration and credentials.
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
//
// # Function Explanation
// 
//	The Put function of AWSCredentialManagementClient creates or updates an AWS credential resource in the AWS plane. If the
//	 credential type is not supported, it returns an error.
func (cpm *AWSCredentialManagementClient) Put(ctx context.Context, credential ucp.AWSCredentialResource) error {
	if strings.EqualFold(*credential.Type, AWSCredential) {
		_, err := cpm.AWSCredentialClient.CreateOrUpdate(ctx, AWSPlaneName, defaultSecretName, credential, nil)
		return err
	}
	return &ErrUnsupportedCloudProvider{}
}

// Get, gets the credential from the provided ucp provider plane
//
// # Function Explanation
// 
//	The Get function of the AWSCredentialManagementClient retrieves the AWS credentials for the given name from the backend 
//	and returns a ProviderCredentialConfiguration object. If the credentials are not found, it returns an empty 
//	ProviderCredentialConfiguration object with the Enabled field set to false. If any other error occurs, it returns an 
//	error.
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
			return ProviderCredentialConfiguration{}, clierrors.Message("Unable to find credentials for cloud provider %s.", name)
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
//
// # Function Explanation
// 
//	The List function of AWSCredentialManagementClient retrieves a list of all AWS credentials and returns them as a slice 
//	of CloudProviderStatus objects. It uses a pager to iterate through the list of credentials and adds them to the 
//	providerList slice. Finally, it creates a slice of CloudProviderStatus objects from the providerList and returns it. If 
//	an error occurs during the iteration, it is returned to the caller.
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
//	The Delete function in AWSCredentialManagementClient attempts to delete a credential from the AWSPlaneName provider 
//	plane. It returns a boolean and an error, with the boolean indicating whether the credential was successfully deleted or
//	 not. If the credential is not found, the function returns true and no error. If an error occurs, the function returns 
//	false and the error.
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
