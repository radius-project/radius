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

	"github.com/radius-project/radius/pkg/cli/clients"
	ucp "github.com/radius-project/radius/pkg/ucp/api/v20220901privatepreview"
)

const (
	AzurePlaneType = "azure"
	AWSPlaneType   = "aws"
)

//go:generate mockgen -destination=./mock_credentialmanagementclient.go -package=credential -self_package github.com/radius-project/radius/pkg/cli/credential github.com/radius-project/radius/pkg/cli/credential CredentialManagementClient

// CredentialManagementClient is used to interface with cloud provider configuration and credentials.
type CredentialManagementClient interface {
	// Get gets the credential registered with the given ucp provider plane.
	Get(ctx context.Context, providerName string) (ProviderCredentialConfiguration, error)
	// List lists the credentials registered with all ucp provider planes.
	List(ctx context.Context) ([]CloudProviderStatus, error)
	// PutAWS registers an AWS credential with the respective ucp provider plane.
	PutAWS(ctx context.Context, credential_config ucp.AwsCredentialResource) error
	// PutAzure registers an Azure credential with the respective ucp provider plane.
	PutAzure(ctx context.Context, credential_config ucp.AzureCredentialResource) error
	// Delete unregisters credential from the given ucp provider plane.
	Delete(ctx context.Context, providerName string) (bool, error)
}

const (
	defaultSecretName = "default"
)

// UCPCredentialManagementClient implements operations to manage credentials on ucp.
type UCPCredentialManagementClient struct {
	AzClient  AzureCredentialManagementClientInterface
	AWSClient AWSCredentialManagementClientInterface
}

var _ CredentialManagementClient = (*UCPCredentialManagementClient)(nil)

// PutAWS registers credentials with the provided credential config
//

// PutAWS function takes in a context and an AWSCredentialResource object and returns an error if the AWSClient.Put call fails.
func (cpm *UCPCredentialManagementClient) PutAWS(ctx context.Context, credential ucp.AwsCredentialResource) error {
	err := cpm.AWSClient.Put(ctx, credential)
	return err
}

// PutAzure registers credentials with the provided credential config
//

// PutAzure takes in a context and an AzureCredentialResource object and attempts to store it through the Azure Client,
// returning an error if unsuccessful.
func (cpm *UCPCredentialManagementClient) PutAzure(ctx context.Context, credential ucp.AzureCredentialResource) error {
	err := cpm.AzClient.Put(ctx, credential)
	return err
}

// Get, gets the credential from the provided ucp provider plane
// We've a single credential configured today for all providers which we name as "default"
// example: If we ask for azure credential, then we will fetch the credential with the name "default" because that is the only
// credential for azure expected in the system.
//

// Get retrieves the credential configuration for the specified cloud provider from the backend, or returns an error if
// the credential is not found or an error occurs. If the credential is not found, an empty credential configuration with
// the cloud provider's status set to disabled is returned.
func (cpm *UCPCredentialManagementClient) Get(ctx context.Context, providerName string) (ProviderCredentialConfiguration, error) {
	var err error
	var cred ProviderCredentialConfiguration
	if strings.EqualFold(providerName, AzureCredential) {
		// We send only the name when getting credentials from backend which we already have access to
		cred, err = cpm.AzClient.Get(ctx, defaultSecretName)
	} else if strings.EqualFold(providerName, AWSCredential) {
		// We send only the name when getting credentials from backend which we already have access to
		cred, err = cpm.AWSClient.Get(ctx, defaultSecretName)
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
//

// List() lists the credentials from both Azure and AWS and returns a slice of CloudProviderStatus. It returns
// an error if either of the list operations fail.
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
// We've a single credential configured today for all providers which we name as "default"
// example: If we ask to delete azure credential, then we will delete the credential with the name "default" because that is the only
// credential for azure expected in the system.
//

// Delete() checks the provider name and calls the appropriate client's Delete function to delete the default
// secret name. It returns a boolean and an error if the provider name is invalid.
func (cpm *UCPCredentialManagementClient) Delete(ctx context.Context, providerName string) (bool, error) {
	if strings.EqualFold(providerName, AzureCredential) {
		return cpm.AzClient.Delete(ctx, defaultSecretName)
	} else if strings.EqualFold(providerName, AWSCredential) {
		return cpm.AWSClient.Delete(ctx, defaultSecretName)
	}

	return true, nil
}
