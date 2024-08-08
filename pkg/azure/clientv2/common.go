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

package clientv2

import (
	"context"
	"fmt"
)

// GetResourceGroupLocation retrieves the location of a given resource group from an Azure subscription. It returns an
// error if the resource group or the subscription cannot be found.
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
