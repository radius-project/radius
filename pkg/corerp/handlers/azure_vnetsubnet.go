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

package handlers

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
	"github.com/radius-project/radius/pkg/azure/armauth"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

func NewAzureVirtualNetworkSubnetHandler(arm *armauth.ArmConfig) ResourceHandler {
	return &azureVirtualNetworkSubnetHandler{arm: arm}
}

type azureVirtualNetworkSubnetHandler struct {
	arm *armauth.ArmConfig
}

func (handler *azureVirtualNetworkSubnetHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	subnet, ok := options.Resource.CreateResource.Data.(*armnetwork.Subnet)
	if !ok {
		return nil, errors.New("cannot parse subnet")
	}

	subID := options.Resource.ID.FindScope("subscriptions")
	resourceGroupName := options.Resource.ID.FindScope("resourceGroups")
	if subID == "" || resourceGroupName == "" {
		return nil, fmt.Errorf("cannot find subscription or resource group in resource ID %s", options.Resource.ID)
	}

	networkClientFactory, err := armnetwork.NewClientFactory(subID, handler.arm.ClientOptions.Cred, nil)
	if err != nil {
		return nil, err
	}
	subnetClient := networkClientFactory.NewSubnetsClient()

	vnetName := options.Resource.ID.Truncate().Name()

	orgPrefix := to.String(subnet.Properties.AddressPrefix)

	var lastErr error
	for retry := 0; retry < 5; retry++ {
		// if subnet.Properties.AddressPrefix is nil, we need to find an available subnet
		if orgPrefix == "" {
			// TODO: for loop until we find an available subnet
			checked := map[int]bool{}
			pager := subnetClient.NewListPager(resourceGroupName, vnetName, nil)
			for pager.More() {
				s, err := pager.NextPage(ctx)
				if err != nil {
					return nil, err
				}
				for _, s := range s.Value {
					if to.String(s.Name) == to.String(subnet.Name) {
						// no ops
						return nil, nil
					}

					ip := strings.Split(to.String(s.Properties.AddressPrefix), ".")
					if ip[0] != "10" {
						continue
					}
					cnt, _ := strconv.ParseInt(ip[2], 10, 32)
					checked[int(cnt)] = true
				}
			}

			subnetPrefix := ""
			for i := 2; i < 253; i++ {
				if !checked[i] {
					subnetPrefix = fmt.Sprintf("10.1.%d.0/24", i)
					break
				}
			}

			if subnetPrefix == "" {
				return nil, errors.New("no available subnet")
			}

			subnet.Properties.AddressPrefix = to.Ptr(subnetPrefix)
		}

		poller, err := subnetClient.BeginCreateOrUpdate(ctx, resourceGroupName, vnetName, to.String(subnet.Name), *subnet, nil)
		if err != nil {
			lastErr = err
			logger.Info("failed to create subnet", "error", err, "retry", retry)
			fmt.Printf("\n\n### failed to create subnet: %s, retry: %d, %s", err.Error(), retry, *subnet.Properties.AddressPrefix)
			time.Sleep(60 * time.Second)
			continue
		}

		_, err = poller.PollUntilDone(ctx, nil)
		if err != nil {
			lastErr = err
			logger.Info("failed to poll subnet update", "error", err, "retry", retry)
			fmt.Printf("\n\n### failed to poll subnet update: %s, retry: %d, %s", err.Error(), retry, *subnet.Properties.AddressPrefix)
			time.Sleep(60 * time.Second)
			continue
		}

		lastErr = nil
		break
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return nil, nil
}

func (handler *azureVirtualNetworkSubnetHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	subID := options.Resource.ID.FindScope("subscriptions")
	resourceGroupName := options.Resource.ID.FindScope("resourceGroups")

	networkClientFactory, err := armnetwork.NewClientFactory(subID, handler.arm.ClientOptions.Cred, nil)
	if err != nil {
		return err
	}
	subnetClient := networkClientFactory.NewSubnetsClient()
	poller, err := subnetClient.BeginDelete(ctx, resourceGroupName, options.Resource.ID.Truncate().Name(), options.Resource.ID.Name(), nil)
	if err != nil {
		return err
	}

	_, err = poller.PollUntilDone(ctx, nil)
	return err
}
