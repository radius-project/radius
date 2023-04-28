// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	"github.com/project-radius/radius/pkg/azure/clientv2"
)

// SubscriptionResult is the result of loading Azure subscriptions for the user.
type SubscriptionResult struct {
	Default       *Subscription
	Subscriptions []Subscription
}

// LoadSubscriptionsFromProfile reads the users Azure profile to find subscription data.
func LoadSubscriptionsFromProfile() (*SubscriptionResult, error) {
	path, err := ProfilePath()
	if err != nil {
		return nil, fmt.Errorf("cannot load subscriptions from profile: %v", err)
	}

	profile, err := LoadProfile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot load subscriptions from profile: %v", err)
	}

	result := &SubscriptionResult{}
	for _, s := range profile.Subscriptions {
		if s.State != "Enabled" {
			continue
		}

		result.Subscriptions = append(result.Subscriptions, s)

		if s.IsDefault {
			result.Default = &s
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
				Name:     *subscription.DisplayName,
				ID:       *subscription.SubscriptionID,
				TenantID: *subscription.TenantID,
			})
		}
	}

	return result, nil
}
