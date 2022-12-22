// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clientv2

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	armruntime "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
)

var defaultClientOptions = &arm.ClientOptions{
	ClientOptions: azcore.ClientOptions{
		Retry: policy.RetryOptions{
			MaxRetries: 10, // TODO: Find the better retry number.
		},
	},
}

// Options represents the client option for azure sdk client including authentication.
type Options struct {
	// Cred represents a credential for OAuth token.
	Cred azcore.TokenCredential

	BaseURI string
}

// NewFederatedIdentityClient creates new federated identity client.
func NewFederatedIdentityClient(subscriptionID string, options *Options) (*armmsi.FederatedIdentityCredentialsClient, error) {
	// TODO: Add LRU cache to maintain the clients.
	return armmsi.NewFederatedIdentityCredentialsClient(subscriptionID, options.Cred, defaultClientOptions)
}

// NewUserAssignedIdentityClient creates new user assigned managed identity client.
func NewUserAssignedIdentityClient(subscriptionID string, options *Options) (*armmsi.UserAssignedIdentitiesClient, error) {
	// TODO: Add LRU cache to maintain the clients.
	return armmsi.NewUserAssignedIdentitiesClient(subscriptionID, options.Cred, defaultClientOptions)
}

// NewCustomActionClient creates an instance of the CustomActionClient.
func NewCustomActionClient(subscriptionID string, options *Options) (*CustomActionClient, error) {
	baseURI := DefaultBaseURI
	if options.BaseURI != "" {
		baseURI = options.BaseURI
	}

	client, err := armresources.NewClient(subscriptionID, options.Cred, defaultClientOptions)
	if err != nil {
		return nil, err
	}

	pipeline, err := armruntime.NewPipeline(ModuleName, ModuleVersion, options.Cred, runtime.PipelineOptions{}, defaultClientOptions)
	if err != nil {
		return nil, err
	}

	return &CustomActionClient{
		client:   client,
		pipeline: &pipeline,
		baseURI:  baseURI,
	}, nil
}
