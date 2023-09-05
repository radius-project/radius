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
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	"github.com/radius-project/radius/pkg/azure/clientv2"
)

// SubscriptionResult is the result of loading Azure subscriptions for the user.
type SubscriptionResult struct {
	Default       *Subscription
	Subscriptions []Subscription
}

// LoadSubscriptionsFromProfile reads the users Azure profile to find subscription data.
//

// LoadSubscriptionsFromProfile() reads the profile from a file, filters out the enabled subscriptions and returns a
// SubscriptionResult object containing the enabled subscriptions and the default subscription. If an error occurs, an
// error is returned.
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
//

// LoadSubscriptionsFromAzure retrieves a list of subscriptions from Azure and returns them in a SubscriptionResult struct.
//
//	It returns an error if there is an issue retrieving the list.
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
