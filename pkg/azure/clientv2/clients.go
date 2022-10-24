// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clientv2

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
)

var defaultClientOptions = &arm.ClientOptions{
	ClientOptions: azcore.ClientOptions{
		Retry: policy.RetryOptions{
			MaxRetries: 10, // TODO: Find the better retry number.
		},
	},
}

// AzureClientOption represents the client option for azure sdk client including authentication.
type AzureClientOption struct {
	// Cred represents a credential for OAuth token.
	Cred azcore.TokenCredential
}

// NewFederatedIdentityClient creates new federated identity client.
func NewFederatedIdentityClient(subscriptionID string, option *AzureClientOption) (*armmsi.FederatedIdentityCredentialsClient, error) {
	// TODO: Add LRU cache to maintain the clients.
	return armmsi.NewFederatedIdentityCredentialsClient(subscriptionID, option.Cred, defaultClientOptions)
}

// NewUserAssignedIdentityClient creates new user assigned managed identity client.
func NewUserAssignedIdentityClient(subscriptionID string, option *AzureClientOption) (*armmsi.UserAssignedIdentitiesClient, error) {
	// TODO: Add LRU cache to maintain the clients.
	return armmsi.NewUserAssignedIdentitiesClient(subscriptionID, option.Cred, defaultClientOptions)
}
