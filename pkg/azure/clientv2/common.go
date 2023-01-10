// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clientv2

import (
	"context"
	"fmt"
)

// GetResourceGroupLocation returns the location of the resource group.
func GetResourceGroupLocation(ctx context.Context, subscriptionID string, resourceGroupName string, options *Options) (*string, error) {
	client, err := NewResourceGroupsClient(subscriptionID, options)
	if err != nil {
		return nil, err
	}

	rg, err := client.Get(ctx, resourceGroupName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource group location: %w", err)
	}

	return rg.Location, nil
}
