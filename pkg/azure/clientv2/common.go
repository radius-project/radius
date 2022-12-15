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
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
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
