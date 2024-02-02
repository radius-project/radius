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

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
	"github.com/radius-project/radius/pkg/azure/armauth"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
)

func NewAzureNSGHandler(arm *armauth.ArmConfig) ResourceHandler {
	return &azureNSGHandler{arm: arm}
}

type azureNSGHandler struct {
	arm *armauth.ArmConfig
}

func (handler *azureNSGHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	nsg, ok := options.Resource.CreateResource.Data.(*armnetwork.SecurityGroup)
	if !ok {
		return nil, errors.New("cannot parse subnet")
	}

	subID := options.Resource.ID.FindScope("subscriptions")
	resourceGroupName := options.Resource.ID.FindScope("resourceGroups")
	if subID == "" || resourceGroupName == "" {
		return nil, fmt.Errorf("cannot find subscription or resource group in resource ID %s", options.Resource.ID)
	}

	publicIP, ok := options.DependencyProperties[rpv1.LocalIDAzurePublicIP]["publicIPAddress"]
	if !ok {
		return nil, errors.New("missing dependency: a user assigned identity is required to create role assignment")
	}

	for i := range nsg.Properties.SecurityRules {
		if to.String(nsg.Properties.SecurityRules[i].Name) == "AllowPublicIPAddress" {
			nsg.Properties.SecurityRules[i].Properties.DestinationAddressPrefix = to.Ptr(publicIP)
		}
	}

	networkClientFactory, err := armnetwork.NewClientFactory(subID, handler.arm.ClientOptions.Cred, nil)
	if err != nil {
		return nil, err
	}
	nsgClient := networkClientFactory.NewSecurityGroupsClient()

	poller, err := nsgClient.BeginCreateOrUpdate(ctx, resourceGroupName, *nsg.Name, *nsg, nil)
	if err != nil {
		return nil, err
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, err
	}

	return map[string]string{}, nil
}

func (handler *azureNSGHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	subID := options.Resource.ID.FindScope("subscriptions")
	resourceGroupName := options.Resource.ID.FindScope("resourceGroups")
	if subID == "" || resourceGroupName == "" {
		return fmt.Errorf("cannot find subscription or resource group in resource ID %s", options.Resource.ID)
	}

	networkClientFactory, err := armnetwork.NewClientFactory(subID, handler.arm.ClientOptions.Cred, nil)
	if err != nil {
		return err
	}

	nsgClient := networkClientFactory.NewSecurityGroupsClient()
	poller, err := nsgClient.BeginDelete(ctx, resourceGroupName, options.Resource.ID.Name(), nil)
	if err != nil {
		return err
	}

	_, err = poller.PollUntilDone(ctx, nil)
	return err
}
