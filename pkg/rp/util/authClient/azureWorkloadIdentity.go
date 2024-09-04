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

package authClient

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/containers/azcontainerregistry"
	"github.com/radius-project/radius/pkg/to"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
)

var _ AuthClient = (*azureWorkloadIdentity)(nil)

type azureWorkloadIdentity struct {
	clientID string
	tenantID string
}

// NewAzureWorkloadIdentity creates a new NewAzureWorkloadIdentity instance.
func NewAzureWorkloadIdentity(clientID string, tenantID string) AuthClient {
	return &azureWorkloadIdentity{clientID: clientID, tenantID: tenantID}
}

// GetAuthClient retrieves an authenticated client for accessing Azure Container Registry (ACR) using
// Azure Workload Identity. It first acquires an Azure Active Directory (AAD) access token by leveraging
// the federated token provided by AKS. The AAD access token is then exchanged for an ACR refresh token.
// The function returns a remote.Client that can authenticate and interact with the container registry
// using the obtained refresh token.
func (wi *azureWorkloadIdentity) GetAuthClient(ctx context.Context, templatePath string) (remote.Client, error) {
	opt := &azidentity.WorkloadIdentityCredentialOptions{
		ClientID: wi.clientID,
		TenantID: wi.tenantID,
	}

	// Get AAD access token by sending projected federated token from AKS
	cred, err := azidentity.NewWorkloadIdentityCredential(opt)
	if err != nil {
		return nil, err
	}

	aadToken, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://containerregistry.azure.net/.default"}})
	if err != nil {
		return nil, err
	}

	registryHost, err := getRegistryHostname(templatePath)
	if err != nil {
		return nil, err
	}

	ac, err := azcontainerregistry.NewAuthenticationClient(fmt.Sprintf("https://%s", registryHost), &azcontainerregistry.AuthenticationClientOptions{})
	if err != nil {
		return nil, err
	}

	// Get refresh token from ACR by exchanging for the above AAD access token
	rt, err := ac.ExchangeAADAccessTokenForACRRefreshToken(ctx, "access_token", registryHost, &azcontainerregistry.AuthenticationClientExchangeAADAccessTokenForACRRefreshTokenOptions{
		AccessToken: to.Ptr(aadToken.Token),
		Tenant:      to.Ptr(wi.tenantID),
	})

	if err != nil {
		return nil, err
	}

	// Return a new auth.Client using the retrieved refresh token for ACR
	return &auth.Client{
		Client: retry.DefaultClient,
		Credential: auth.StaticCredential(registryHost, auth.Credential{
			RefreshToken: *rt.RefreshToken,
		}),
	}, nil
}
