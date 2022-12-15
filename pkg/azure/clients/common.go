// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clients

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/azure/armauth"
)

func GetResourceGroupLocation(ctx context.Context, armConfig armauth.ArmConfig, subscriptionID string, resourceGroupName string) (*string, error) {
	rgc := NewGroupsClient(subscriptionID, armConfig.Auth)

	resourceGroup, err := rgc.Get(ctx, resourceGroupName)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource group location: %w", err)
	}

	return resourceGroup.Location, nil
}
