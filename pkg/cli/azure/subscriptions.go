// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	"github.com/Azure/go-autorest/autorest/azure/cli"
	"github.com/project-radius/radius/pkg/azure/clientv2"
)

// SubscriptionResult is the result of loading Azure subscriptions for the user.
type SubscriptionResult struct {
	Default       *Subscription
	Subscriptions []Subscription
}

// Subscription represents a subscription the user has access to.
type Subscription struct {
	DisplayName    string
	SubscriptionID string
	TenantID       string
}

// LoadSubscriptionsFromProfile reads the users Azure profile to find subscription data.
func LoadSubscriptionsFromProfile() (*SubscriptionResult, error) {
	path, err := cli.ProfilePath()
	if err != nil {
		return nil, fmt.Errorf("cannot load subscriptions from profile: %v", err)
	}

	profile, err := cli.LoadProfile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot load subscriptions from profile: %v", err)
	}

	result := &SubscriptionResult{}
	for _, s := range profile.Subscriptions {
		if s.State != "Enabled" {
			continue
		}

		sub := Subscription{
			DisplayName:    s.Name,
			SubscriptionID: s.ID,
			TenantID:       s.TenantID,
		}
		result.Subscriptions = append(result.Subscriptions, sub)

		if s.IsDefault {
			result.Default = &sub
		}
	}

	return result, nil
}

// LoadSubscriptionsFromAzure uses ARM to find subscription data.
func LoadSubscriptionsFromAzure(ctx context.Context, options clientv2.Options) (*SubscriptionResult, error) {
	client, err := clientv2.NewSubscriptionsClient(&options)
	if err != nil {
		return nil, err
	}

	pager := client.NewListPager(&armsubscriptions.ClientListOptions{})
	result := &SubscriptionResult{}
	for pager.More() {
		nextPage, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, subscription := range nextPage.SubscriptionListResult.Value {
			result.Subscriptions = append(result.Subscriptions, Subscription{
				DisplayName:    *subscription.DisplayName,
				SubscriptionID: *subscription.SubscriptionID,
				// We don't get the tenant ID in this API call - we can do it later when its needed.
				// This way we avoid doing an N+1 query for data we won't need for each sub.
				TenantID: "",
			})
		}
	}

	return result, nil
}
