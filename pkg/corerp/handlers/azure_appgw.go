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
)

func NewAzureAppGWHandler(arm *armauth.ArmConfig) ResourceHandler {
	return &azureAppGWHandler{arm: arm}
}

type azureAppGWHandler struct {
	arm *armauth.ArmConfig
}

func (handler *azureAppGWHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	appgw, ok := options.Resource.CreateResource.Data.(*armnetwork.ApplicationGateway)
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
	cli := networkClientFactory.NewApplicationGatewaysClient()

	poller, err := cli.BeginCreateOrUpdate(ctx, resourceGroupName, *appgw.Name, *appgw, nil)
	if err != nil {
		return nil, err
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, err
	}

	return map[string]string{}, nil
}

func (handler *azureAppGWHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	subID := options.Resource.ID.FindScope("subscriptions")
	resourceGroupName := options.Resource.ID.FindScope("resourceGroups")
	if subID == "" || resourceGroupName == "" {
		return fmt.Errorf("cannot find subscription or resource group in resource ID %s", options.Resource.ID)
	}

	networkClientFactory, err := armnetwork.NewClientFactory(subID, handler.arm.ClientOptions.Cred, nil)
	if err != nil {
		return err
	}

	cli := networkClientFactory.NewApplicationGatewaysClient()
	poller, err := cli.BeginDelete(ctx, resourceGroupName, options.Resource.ID.Name(), nil)
	if err != nil {
		return err
	}

	_, err = poller.PollUntilDone(ctx, nil)
	return err
}
