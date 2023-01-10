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
	armauthorization "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	armservicebus "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/servicebus/armservicebus/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
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

// NewSubscriptionsClient creates a new generic client to handle subscriptions.
func NewSubscriptionsClient(options *Options) (*armsubscriptions.Client, error) {
	return armsubscriptions.NewClient(options.Cred, defaultClientOptions)
}

// NewGenericResourceClient creates a new generic client to handle resources.
func NewGenericResourceClient(subscriptionID string, options *Options) (*armresources.Client, error) {
	return armresources.NewClient(subscriptionID, options.Cred, defaultClientOptions)
}

// NewAccountsClient creates a new accounts client to handle storage accounts.
func NewAccountsClient(subscriptionID string, options *Options) (*armstorage.AccountsClient, error) {
	return armstorage.NewAccountsClient(subscriptionID, options.Cred, defaultClientOptions)
}

// NewRoleDefinitionsClient creates a new role definitions client to handle role definitions.
func NewRoleDefinitionsClient(options *Options) (*armauthorization.RoleDefinitionsClient, error) {
	return armauthorization.NewRoleDefinitionsClient(options.Cred, defaultClientOptions)
}

// NewRoleAssignmentsClient creates a new role assignments client to handle role assignments.
func NewRoleAssignmentsClient(subscriptionID string, options *Options) (*armauthorization.RoleAssignmentsClient, error) {
	return armauthorization.NewRoleAssignmentsClient(subscriptionID, options.Cred, defaultClientOptions)
}

// NewServiceBusNamespacesClient creates a new service bus namespaces client to handle service bus namespaces.
func NewServiceBusNamespacesClient(subscriptionID string, options *Options) (*armservicebus.NamespacesClient, error) {
	return armservicebus.NewNamespacesClient(subscriptionID, options.Cred, defaultClientOptions)
}

// NewDeploymentsClient creates a new deployments client to handle deployments.
func NewDeploymentsClient(subscriptionID string, options *Options) (*armresources.DeploymentsClient, error) {
	return armresources.NewDeploymentsClient(subscriptionID, options.Cred, defaultClientOptions)
}

// NewResourceGroupsClient creates a new resource groups client to handle resource groups.
func NewResourceGroupsClient(subscriptionID string, options *Options) (*armresources.ResourceGroupsClient, error) {
	return armresources.NewResourceGroupsClient(subscriptionID, options.Cred, defaultClientOptions)
}
