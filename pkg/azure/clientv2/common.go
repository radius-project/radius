// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clientv2

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
)

// GetResourceGroupLocation returns the location of the resource group.
func GetResourceGroupLocation(ctx context.Context, credential azcore.TokenCredential, subscriptionID string, resourceGroupName string) (*string, error) {
	client, err := armresources.NewResourceGroupsClient(subscriptionID, credential, &arm.ClientOptions{})
	if err != nil {
		return nil, err
	}

	if err != nil {
		return nil, err
	}

	rg, err := client.Get(ctx, resourceGroupName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource group location: %w", err)
	}

	return rg.Location, nil
}

// GetSubscriptions returns the list of subscriptions.
func GetSubscriptions(ctx context.Context, credential azcore.TokenCredential, options *arm.ClientOptions) (*[]*armsubscriptions.Subscription, error) {
	// If the credential is nil, get the default one and try.
	if credential == nil {
		var err error
		credential, err = azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			return nil, err
		}
	}

	client, err := armsubscriptions.NewClient(credential, options)
	if err != nil {
		return nil, err
	}

	pager := client.NewListPager(&armsubscriptions.ClientListOptions{})
	subscriptions := make([]*armsubscriptions.Subscription, 0)
	for pager.More() {
		nextPage, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		subscriptions = append(subscriptions, nextPage.SubscriptionListResult.Value...)
	}

	return &subscriptions, nil
}
