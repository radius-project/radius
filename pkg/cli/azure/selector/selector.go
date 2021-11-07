// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package selector

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/radius/pkg/azure/clients"
	radazure "github.com/Azure/radius/pkg/cli/azure"
	"github.com/Azure/radius/pkg/cli/output"
	"github.com/Azure/radius/pkg/cli/prompt"
)

var SupportedLocations = [5]string{
	"australiaeast",
	"eastus",
	"northeurope",
	"westeurope",
	"westus2",
}

func Select(ctx context.Context) (string, string, error) {
	authorizer, err := auth.NewAuthorizerFromCLI()
	if err != nil {
		return "", "", err
	}

	sub, err := selectSubscription(ctx, authorizer)
	if err != nil {
		return "", "", err
	}

	resourceGroup, err := selectResourceGroup(ctx, authorizer, sub)
	if err != nil {
		return "", "", err
	}

	return sub.SubscriptionID, resourceGroup, nil
}

func selectSubscription(ctx context.Context, authorizer autorest.Authorizer) (radazure.Subscription, error) {
	subs, err := radazure.LoadSubscriptionsFromProfile()
	if err != nil {
		// Failed to load subscriptions from the user profile, fall back to online.
		subs, err = radazure.LoadSubscriptionsFromAzure(ctx, authorizer)
		if err != nil {
			return radazure.Subscription{}, err
		}
	}

	if subs.Default != nil {
		confirmed, err := prompt.Confirm(fmt.Sprintf("Use Subscription '%v' [y/n]?", subs.Default.DisplayName))
		if err != nil {
			return radazure.Subscription{}, err
		}

		if confirmed {
			return *subs.Default, nil
		}
	}

	// build prompt to select from list
	names := []string{}
	for _, s := range subs.Subscriptions {
		names = append(names, s.DisplayName)
	}

	index, err := prompt.Select("Select Subscription:", names)
	if err != nil {
		return radazure.Subscription{}, err
	}

	return subs.Subscriptions[index], nil
}

func selectResourceGroup(ctx context.Context, authorizer autorest.Authorizer, sub radazure.Subscription) (string, error) {
	rgc := clients.NewGroupsClient(sub.SubscriptionID, authorizer)

	name, err := prompt.Text("Enter a Resource Group name:", prompt.EmptyValidator)
	if err != nil {
		return "", err
	}

	resp, err := rgc.CheckExistence(ctx, name)
	if err != nil {
		return "", err
	} else if !resp.HasHTTPStatus(404) {
		// already exists
		return name, nil
	}

	output.LogInfo("Resource Group '%v' will be created...", name)

	subc := clients.NewSubscriptionClient(authorizer)

	locations, err := subc.ListLocations(ctx, sub.SubscriptionID)
	if err != nil {
		return "", fmt.Errorf("cannot list locations: %w", err)
	}

	displayNames := []string{}
	displayNameToProgrammaticName := map[string]string{}
	for _, loc := range *locations.Value {
		if !IsSupportedLocation(*loc.Name) {
			continue
		}

		// Use the display name for the prompt
		displayNames = append(displayNames, *loc.DisplayName)
		displayNameToProgrammaticName[*loc.DisplayName] = *loc.Name
	}

	// alphabetize so the list is stable and scannable
	sort.Strings(displayNames)

	index, err := prompt.Select("Select a location:", displayNames)
	if err != nil {
		return "", err
	}

	selected := displayNames[index]
	programmaticName := displayNameToProgrammaticName[selected]
	_, err = rgc.CreateOrUpdate(ctx, name, resources.Group{
		Location: to.StringPtr(programmaticName),
	})
	if err != nil {
		return "", err
	}

	return name, nil
}

// operates on the *programmatic* location name ('eastus' not 'East US')
func IsSupportedLocation(name string) bool {
	for _, loc := range SupportedLocations {
		if strings.EqualFold(name, loc) {
			return true
		}
	}

	return false
}
