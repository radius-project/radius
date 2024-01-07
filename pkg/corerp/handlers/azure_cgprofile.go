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

	cs2client "github.com/radius-project/azure-cs2/client/v20230515preview"
	"github.com/radius-project/radius/pkg/azure/armauth"
	"github.com/radius-project/radius/pkg/azure/clientv2"
)

func NewAzureCGProfileHandler(arm *armauth.ArmConfig) ResourceHandler {
	return &azureCGProfileHandler{arm: arm}
}

type azureCGProfileHandler struct {
	arm *armauth.ArmConfig
}

func (handler *azureCGProfileHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	profile, ok := options.Resource.CreateResource.Data.(*cs2client.ContainerGroupProfile)
	if !ok {
		return nil, errors.New("cannot parse container group profile")
	}

	subID := options.Resource.ID.FindScope("subscriptions")
	resourceGroupName := options.Resource.ID.FindScope("resourceGroups")
	if subID == "" || resourceGroupName == "" {
		return nil, fmt.Errorf("cannot find subscription or resource group in resource ID %s", options.Resource.ID)
	}

	// Ensure resource group is created.
	err := clientv2.EnsureResourceGroupIsCreated(ctx, subID, resourceGroupName, "West US 3", &handler.arm.ClientOptions)
	if err != nil {
		return nil, err
	}

	cgp, err := cs2client.NewContainerGroupProfilesClient(subID, handler.arm.ClientOptions.Cred, nil)
	if err != nil {
		return nil, err
	}

	resp, err := cgp.CreateOrUpdate(ctx, resourceGroupName, *profile.Name, *profile, nil)
	if err != nil {
		return nil, err
	}

	properties := map[string]string{
		"containerGroupProfileID": *resp.ID,
	}

	return properties, nil
}

func (handler *azureCGProfileHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	subID := options.Resource.ID.FindScope("subscriptions")
	resourceGroupName := options.Resource.ID.FindScope("resourceGroups")

	cgp, err := cs2client.NewContainerGroupProfilesClient(subID, handler.arm.ClientOptions.Cred, nil)
	if err != nil {
		return err
	}
	_, err = cgp.Delete(ctx, resourceGroupName, options.Resource.ID.Name(), nil)
	return err
}
