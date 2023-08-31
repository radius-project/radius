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

package azure

import (
	"context"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	"github.com/radius-project/radius/pkg/azure/armauth"
)

//go:generate mockgen -destination=./client_mock.go -package=azure -self_package github.com/radius-project/radius/pkg/cli/azure github.com/radius-project/radius/pkg/cli/azure Client

// Client is an interface that abstracts `rad init`'s interactions with Azure. This is for testing purposes.
type Client interface {
	// Locations lists the locations available for a subscription.
	Locations(ctx context.Context, subscriptionID string) ([]armsubscriptions.Location, error)
	// Subscriptions lists the subscriptions available to the user.
	Subscriptions(ctx context.Context) (*SubscriptionResult, error)
	// ResourceGroups lists the existing resource groups in a subscription.
	ResourceGroups(ctx context.Context, subscriptionID string) ([]armresources.ResourceGroup, error)
	// CheckResourceGroupExistence checks if a resource group exists.
	CheckResourceGroupExistence(ctx context.Context, subscriptionID string, resourceGroupName string) (bool, error)
	// CreateOrUpdateResourceGroup creates or updates a resource group.
	CreateOrUpdateResourceGroup(ctx context.Context, subscriptionID string, resourceGroupName string, location string) error
}

// NewClient returns a new Client.
func NewClient() Client {
	return &client{}
}

type client struct {
	config       *armauth.ArmConfig
	configLoader sync.Once
}

var _ Client = &client{}

func (c *client) initializeCredentials() error {
	var err error
	c.configLoader.Do(func() {
		c.config, err = armauth.NewArmConfig(nil)
	})

	return err
}

// Locations lists the locations available for a subscription.
//

// Locations() initializes credentials, creates a new client, and then uses a pager to retrieve a
// list of locations associated with a given subscription ID, which is then returned as a slice of
// armsubscriptions.Location objects. It returns an error if any of the steps fail.
func (c *client) Locations(ctx context.Context, subscriptionID string) ([]armsubscriptions.Location, error) {
	err := c.initializeCredentials()
	if err != nil {
		return nil, err
	}

	// Use the display name for the prompt
	// alphabetize so the list is stable and scannable
	client, err := armsubscriptions.NewClient(c.config.ClientOptions.Cred, nil)
	if err != nil {
		return nil, err
	}

	pager := client.NewListLocationsPager(subscriptionID, nil)
	locations := []armsubscriptions.Location{}
	for pager.More() {
		next, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, location := range next.Value {
			locations = append(locations, *location)
		}
	}

	return locations, nil
}

// Subscriptions lists the subscriptions available to the user.
//

// Subscriptions() attempts to load subscriptions from the user profile, and if that fails, it falls back to
// loading them from Azure, returning an error if unsuccessful.
func (c *client) Subscriptions(ctx context.Context) (*SubscriptionResult, error) {
	err := c.initializeCredentials()
	if err != nil {
		return nil, err
	}

	subs, err := LoadSubscriptionsFromProfile()
	if err != nil {
		// Failed to load subscriptions from the user profile, fall back to online.
		subs, err = LoadSubscriptionsFromAzure(ctx, c.config.ClientOptions)
		if err != nil {
			return nil, err
		}
	}

	return subs, nil
}

// ResourceGroups lists the existing resource groups in a subscription.
//

// ResourceGroups() initializes credentials, creates a new resource groups client, and then iterates through a list of
// resource groups, appending each one to a slice before returning the slice and any errors.
func (c *client) ResourceGroups(ctx context.Context, subscriptionID string) ([]armresources.ResourceGroup, error) {
	err := c.initializeCredentials()
	if err != nil {
		return nil, err
	}

	client, err := armresources.NewResourceGroupsClient(subscriptionID, c.config.ClientOptions.Cred, nil)
	if err != nil {
		return nil, err
	}

	pager := client.NewListPager(nil)
	groups := []armresources.ResourceGroup{}
	for pager.More() {
		next, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, group := range next.Value {
			groups = append(groups, *group)
		}
	}

	return groups, nil
}

// CheckResourceGroupExistence checks if a resource group exists.
//

// CheckResourceGroupExistence initializes credentials, creates a new resource group client, checks the existence of the
// resource group and returns a boolean indicating whether the resource group exists or not. An error is returned if any
// of the steps fail.
func (c *client) CheckResourceGroupExistence(ctx context.Context, subscriptionID string, resourceGroupName string) (bool, error) {
	err := c.initializeCredentials()
	if err != nil {
		return false, err
	}

	client, err := armresources.NewResourceGroupsClient(subscriptionID, c.config.ClientOptions.Cred, nil)
	if err != nil {
		return false, err
	}

	response, err := client.CheckExistence(ctx, resourceGroupName, nil)
	if err != nil {
		return false, err
	}

	return response.Success, nil
}

// CreateOrUpdateResourceGroup creates or updates a resource group.
//

// CreateOrUpdateResourceGroup initializes credentials, creates a new resource group client, creates a new resource group
// with the given name and location, and then creates or updates the resource group. An error is returned if any
// of the steps fail.
func (c *client) CreateOrUpdateResourceGroup(ctx context.Context, subscriptionID string, resourceGroupName string, location string) error {
	err := c.initializeCredentials()
	if err != nil {
		return err
	}

	client, err := armresources.NewResourceGroupsClient(subscriptionID, c.config.ClientOptions.Cred, nil)
	if err != nil {
		return err
	}

	group := armresources.ResourceGroup{
		Name:     &resourceGroupName,
		Location: &location,
	}

	_, err = client.CreateOrUpdate(ctx, resourceGroupName, group, nil)
	if err != nil {
		return err
	}

	return nil
}
