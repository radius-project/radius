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

// AzureClientOption represents the client option for azure sdk client including authentication.
type AzureClientOption struct {
	// Cred represents a credential for OAuth token.
	Cred azcore.TokenCredential
}

// NewFederatedIdentityClient creates new federated identity client.
func NewFederatedIdentityClient(subscriptionID string, option *AzureClientOption) (*armmsi.FederatedIdentityCredentialsClient, error) {
	opt := &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Retry: policy.RetryOptions{
				MaxRetries: 10,
			},
		},
	}

	return armmsi.NewFederatedIdentityCredentialsClient(subscriptionID, option.Cred, opt)
}
