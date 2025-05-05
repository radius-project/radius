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

	armpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	armruntime "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/runtime"
	azruntime "github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/radius-project/radius/pkg/azure/armauth"
	cgclient "github.com/radius-project/radius/pkg/sdk/v20241101preview"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

func NewAzureCGProfileHandler(arm *armauth.ArmConfig) ResourceHandler {
	return &azureCGProfileHandler{arm: arm}
}

type azureCGProfileHandler struct {
	arm *armauth.ArmConfig
}

func (handler *azureCGProfileHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	profile, ok := options.Resource.CreateResource.Data.(*cgclient.ContainerGroupProfile)
	if !ok {
		return nil, errors.New("cannot parse container group profile")
	}

	subID := options.Resource.ID.FindScope("subscriptions")
	resourceGroupName := options.Resource.ID.FindScope("resourceGroups")
	if subID == "" || resourceGroupName == "" {
		return nil, fmt.Errorf("cannot find subscription or resource group in resource ID %s", options.Resource.ID)
	}

	pl, err := armruntime.NewPipeline("github.com/radius-project/radius", "v0.0.1", handler.arm.ClientOptions.Cred, azruntime.PipelineOptions{}, &armpolicy.ClientOptions{})
	if err != nil {
		return nil, err
	}

	cgp := cgclient.NewCGProfileClient(subID, pl)

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
	logger := ucplog.FromContextOrDiscard(ctx)
	subID := options.Resource.ID.FindScope("subscriptions")
	resourceGroupName := options.Resource.ID.FindScope("resourceGroups")

	pl, err := armruntime.NewPipeline("github.com/radius-project/radius", "v0.0.1", handler.arm.ClientOptions.Cred, azruntime.PipelineOptions{}, &armpolicy.ClientOptions{})
	if err != nil {
		return err
	}
	cgp := cgclient.NewCGProfileClient(subID, pl)

	logger.Info("deleting container group profile...")
	poller, err := cgp.BeginDelete(ctx, resourceGroupName, options.Resource.ID.Name(), nil)
	if err != nil {
		return err
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return err
	}

	logger.Info("completed deleting container group profile...")
	return nil
}
