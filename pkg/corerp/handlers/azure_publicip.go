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
	"github.com/radius-project/radius/pkg/to"
)

func NewAzurePublicIPHandler(arm *armauth.ArmConfig) ResourceHandler {
	return &azurePublicIPHandler{arm: arm}
}

type azurePublicIPHandler struct {
	arm *armauth.ArmConfig
}

func (handler *azurePublicIPHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	publicIP, ok := options.Resource.CreateResource.Data.(*armnetwork.PublicIPAddress)
	if !ok {
		return nil, errors.New("cannot parse public IP address")
	}

	subID := options.Resource.ID.FindScope("subscriptions")
	resourceGroupName := options.Resource.ID.FindScope("resourceGroups")
	if subID == "" || resourceGroupName == "" {
		return nil, fmt.Errorf("cannot find subscription or resource group in resource ID %s", options.Resource.ID)
	}

	location, err := GetResourceGroupLocation(ctx, handler.arm.ClientOptions, subID, resourceGroupName)
	if err != nil {
		return nil, fmt.Errorf("cannot find resource group location: %w", err)
	}
	publicIP.Location = to.Ptr(location)

	networkClientFactory, err := armnetwork.NewClientFactory(subID, handler.arm.ClientOptions.Cred, nil)
	if err != nil {
		return nil, err
	}

	publicIPAddressesClient := networkClientFactory.NewPublicIPAddressesClient()
	publicIPAddress := ""
	fqdn := ""
	resp, err := publicIPAddressesClient.Get(ctx, resourceGroupName, to.String(publicIP.Name), nil)
	if err == nil {
		publicIPAddress = to.String(resp.Properties.IPAddress)
		fqdn = to.String(resp.Properties.DNSSettings.Fqdn)
	} else {
		pollerIP, err := publicIPAddressesClient.BeginCreateOrUpdate(
			ctx,
			resourceGroupName,
			to.String(publicIP.Name),
			*publicIP,
			nil,
		)
		if err != nil {
			return nil, err
		}

		puIPResp, err := pollerIP.PollUntilDone(ctx, nil)
		if err != nil {
			return nil, err
		}
		publicIPAddress = to.String(puIPResp.Properties.IPAddress)
		fqdn = to.String(puIPResp.Properties.DNSSettings.Fqdn)
	}

	if fqdn == "" {
		fqdn = publicIPAddress
	}

	properties := map[string]string{
		"publicIPAddress": publicIPAddress,
		"publicIPFQDN":    fqdn,
	}

	return properties, nil
}

func (handler *azurePublicIPHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	subID := options.Resource.ID.FindScope("subscriptions")
	resourceGroupName := options.Resource.ID.FindScope("resourceGroups")
	if subID == "" || resourceGroupName == "" {
		return fmt.Errorf("cannot find subscription or resource group in resource ID %s", options.Resource.ID)
	}

	networkClientFactory, err := armnetwork.NewClientFactory(subID, handler.arm.ClientOptions.Cred, nil)
	if err != nil {
		return err
	}

	publicIPAddressesClient := networkClientFactory.NewPublicIPAddressesClient()
	poller, err := publicIPAddressesClient.BeginDelete(ctx, resourceGroupName, options.Resource.ID.Name(), nil)
	if err != nil {
		return err
	}

	_, err = poller.PollUntilDone(ctx, nil)
	return err
}
