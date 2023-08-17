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
			MaxRetries: 10,
		},
	},
}

// Options represents the client option for azure sdk client including authentication.
type Options struct {
	// Cred represents a credential for OAuth token.
	Cred azcore.TokenCredential

	// BaseURI represents the base URI for the client.
	BaseURI string
}

// NewFederatedIdentityClient creates a new FederatedIdentityCredentialsClient and returns it along with any error.
func NewFederatedIdentityClient(subscriptionID string, options *Options) (*armmsi.FederatedIdentityCredentialsClient, error) {
	// TODO: Add LRU cache to maintain the clients.
	return armmsi.NewFederatedIdentityCredentialsClient(subscriptionID, options.Cred, defaultClientOptions)
}

// NewUserAssignedIdentityClient creates a new UserAssignedIdentitiesClient with the given subscriptionID and Options and
// returns it, or an error if one occurs.
func NewUserAssignedIdentityClient(subscriptionID string, options *Options) (*armmsi.UserAssignedIdentitiesClient, error) {
	// TODO: Add LRU cache to maintain the clients.
	return armmsi.NewUserAssignedIdentitiesClient(subscriptionID, options.Cred, defaultClientOptions)
}

// NewCustomActionClient creates a new CustomActionClient with the provided subscriptionID and Options, and returns an
// error if one occurs.
func NewCustomActionClient(subscriptionID string, options *Options, clientOptions *arm.ClientOptions) (*CustomActionClient, error) {
	baseURI := DefaultBaseURI
	if options.BaseURI != "" {
		baseURI = options.BaseURI
	}

	if clientOptions == nil {
		clientOptions = defaultClientOptions
	}

	client, err := armresources.NewClient(subscriptionID, options.Cred, clientOptions)
	if err != nil {
		return nil, err
	}

	pipeline, err := armruntime.NewPipeline(ModuleName, ModuleVersion, options.Cred, runtime.PipelineOptions{}, clientOptions)
	if err != nil {
		return nil, err
	}

	return &CustomActionClient{
		client:   client,
		pipeline: &pipeline,
		baseURI:  baseURI,
	}, nil
}

// NewSubscriptionsClient creates a new ARM Subscriptions Client using the provided options and returns it, or an error if one occurs.
func NewSubscriptionsClient(options *Options) (*armsubscriptions.Client, error) {
	return armsubscriptions.NewClient(options.Cred, defaultClientOptions)
}

// NewGenericResourceClient creates a new ARM resources client with the given subscription ID, options and client options.
func NewGenericResourceClient(subscriptionID string, options *Options, clientOptions *arm.ClientOptions) (*armresources.Client, error) {
	// Allow setting client options for testing.
	if clientOptions == nil {
		clientOptions = defaultClientOptions
	}
	return armresources.NewClient(subscriptionID, options.Cred, clientOptions)
}

// NewProvidersClient creates a new ARM ProvidersClient with the given subscription ID and ARM ClientOptions,
// that can be used to look up resource providers and API versions.
func NewProvidersClient(subcriptionID string, options *Options, clientOptions *arm.ClientOptions) (*armresources.ProvidersClient, error) {
	// Allow setting client options for testing.
	if clientOptions == nil {
		clientOptions = defaultClientOptions
	}
	return armresources.NewProvidersClient(subcriptionID, options.Cred, clientOptions)
}

// NewAccountsClient creates a new ARM Storage Accounts Client with the given subscription ID and options.
func NewAccountsClient(subscriptionID string, options *Options) (*armstorage.AccountsClient, error) {
	return armstorage.NewAccountsClient(subscriptionID, options.Cred, defaultClientOptions)
}

// NewRoleDefinitionsClient creates a new RoleDefinitionsClient from the given Options and returns it, or an error if one occurs.
func NewRoleDefinitionsClient(options *Options) (*armauthorization.RoleDefinitionsClient, error) {
	return armauthorization.NewRoleDefinitionsClient(options.Cred, defaultClientOptions)
}

// NewRoleAssignmentsClient creates a new RoleAssignmentsClient with the given subscriptionID and Options, and returns an
// error if one occurs.
func NewRoleAssignmentsClient(subscriptionID string, options *Options) (*armauthorization.RoleAssignmentsClient, error) {
	return armauthorization.NewRoleAssignmentsClient(subscriptionID, options.Cred, defaultClientOptions)
}

// NewServiceBusNamespacesClient creates a new ARM Service Bus Namespaces Client with the given subscription ID and
// options.
func NewServiceBusNamespacesClient(subscriptionID string, options *Options) (*armservicebus.NamespacesClient, error) {
	return armservicebus.NewNamespacesClient(subscriptionID, options.Cred, defaultClientOptions)
}

// // NewDeploymentsClient creates a new DeploymentsClient which can be used to manage Azure deployments.
func NewDeploymentsClient(subscriptionID string, options *Options) (*armresources.DeploymentsClient, error) {
	return armresources.NewDeploymentsClient(subscriptionID, options.Cred, defaultClientOptions)
}

// NewDeploymentOperationsClient creates a new ARM deployment operations client for managing deployment operations.
func NewDeploymentOperationsClient(subscriptionID string, options *Options) (*armresources.DeploymentOperationsClient, error) {
	return armresources.NewDeploymentOperationsClient(subscriptionID, options.Cred, defaultClientOptions)
}

// NewResourceGroupsClient creates a new ARM Resource Groups Client with the given subscription ID and options.
func NewResourceGroupsClient(subscriptionID string, options *Options) (*armresources.ResourceGroupsClient, error) {
	return armresources.NewResourceGroupsClient(subscriptionID, options.Cred, defaultClientOptions)
}
