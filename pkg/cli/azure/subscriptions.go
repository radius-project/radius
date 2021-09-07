// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"context"
	"fmt"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/cli"
	"github.com/Azure/radius/pkg/azure/clients"
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
func LoadSubscriptionsFromProfile() (SubscriptionResult, error) {
	path, err := cli.ProfilePath()
	if err != nil {
		return SubscriptionResult{}, fmt.Errorf("cannot load subscriptions from profile: %v", err)
	}

	profile, err := cli.LoadProfile(path)
	if err != nil {
		return SubscriptionResult{}, fmt.Errorf("cannot load subscriptions from profile: %v", err)
	}

	result := SubscriptionResult{}
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
func LoadSubscriptionsFromAzure(ctx context.Context, authorizer autorest.Authorizer) (SubscriptionResult, error) {
	subc := clients.NewSubscriptionClient(authorizer)

	// ARM doesn't have the concept of a "default" subscription so we skip it here.
	result := SubscriptionResult{}

	res, err := subc.List(ctx)
	if err != nil {
		return SubscriptionResult{}, fmt.Errorf("cannot load subscriptions from Azure: %v", err)
	}

	// buffer subscriptions into a slice so we can do multiple passes
	for {
		for _, s := range res.Values() {
			sub := Subscription{
				DisplayName:    *s.DisplayName,
				SubscriptionID: *s.SubscriptionID,

				// We don't get the tenant ID in this API call - we can do it later when its needed.
				// This way we avoid doing an N+1 query for data we won't need for each sub.
				TenantID: "",
			}
			result.Subscriptions = append(result.Subscriptions, sub)
		}

		if !res.NotDone() {
			break
		}

		err = res.NextWithContext(ctx)
		if err != nil {
			return SubscriptionResult{}, fmt.Errorf("cannot load subscriptions from Azure: %v", err)
		}
	}

	return result, nil
}
