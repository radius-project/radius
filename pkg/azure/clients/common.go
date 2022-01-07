// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clients

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/go-autorest/autorest"
	"github.com/project-radius/radius/pkg/azure/armauth"
)

func GetDefaultAPIVersion(ctx context.Context, subscriptionId string, authorizer autorest.Authorizer, resourceType string) (string, error) {
	parts := strings.Split(resourceType, "/")
	provider := parts[0]
	typeWithoutProvider := parts[1]

	providerc := NewProvidersClient(subscriptionId, authorizer)

	p, err := providerc.Get(ctx, provider, "")
	if err != nil {
		return "", err
	}

	// For preview API versions, the DefaultAPIVersion isn't set,
	// so get the first from the list of APIVersions instead.
	for _, rt := range *p.ResourceTypes {
		if strings.EqualFold(*rt.ResourceType, typeWithoutProvider) {
			if rt.DefaultAPIVersion != nil {
				return *rt.DefaultAPIVersion, nil
			} else if rt.APIVersions != nil && len(*rt.APIVersions) > 0 {
				return (*rt.APIVersions)[0], nil
			} else {
				return "", fmt.Errorf("no valid api version for type %s", resourceType)
			}
		}
	}

	return "", nil // unreachable
}

func GetResourceGroupLocation(ctx context.Context, armConfig armauth.ArmConfig) (*string, error) {
	rgc := NewGroupsClient(armConfig.SubscriptionID, armConfig.Auth)

	resourceGroup, err := rgc.Get(ctx, armConfig.ResourceGroup)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource group location: %w", err)
	}

	return resourceGroup.Location, nil
}
