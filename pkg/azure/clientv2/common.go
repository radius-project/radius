// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clientv2

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
)

// GetResourceGroupLocation returns the location of the resource group.
func GetResourceGroupLocation(ctx context.Context, subscriptionID string, resourceGroupName string, options *Options) (*string, error) {
	client, err := armresources.NewResourceGroupsClient(subscriptionID, options.Cred, &arm.ClientOptions{})
	if err != nil {
		return nil, err
	}

	rg, err := client.Get(ctx, resourceGroupName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource group location: %w", err)
	}

	return rg.Location, nil
}
