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
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	ngroupsclient "github.com/radius-project/radius/pkg/sdk/v20241101preview"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

func NewAzureCGNGroupsHandler(arm *armauth.ArmConfig) ResourceHandler {
	return &azureCGNGroupsHandler{arm: arm}
}

type azureCGNGroupsHandler struct {
	arm *armauth.ArmConfig
}

func (handler *azureCGNGroupsHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	nGroup, ok := options.Resource.CreateResource.Data.(*ngroupsclient.NGroup)
	if !ok {
		return nil, errors.New("cannot parse container group profile")
	}

	subID := options.Resource.ID.FindScope("subscriptions")
	resourceGroupName := options.Resource.ID.FindScope("resourceGroups")
	if subID == "" || resourceGroupName == "" {
		return nil, fmt.Errorf("cannot find subscription or resource group in resource ID %s", options.Resource.ID)
	}

	cgpID, ok := options.DependencyProperties[rpv1.LocalIDAzureCGProfile]["containerGroupProfileID"]
	if !ok {
		return nil, errors.New("missing dependency: a user assigned identity is required to create role assignment")
	}

	nGroup.Properties.ContainerGroupProfiles[0].Resource.ID = to.Ptr(cgpID)
	logger.Info("ngroup ID: ", nGroup.Properties.ContainerGroupProfiles[0].Resource.ID)
	pl, err := armruntime.NewPipeline("github.com/radius-project/radius", "v0.0.1", handler.arm.ClientOptions.Cred, azruntime.PipelineOptions{}, &armpolicy.ClientOptions{})
	if err != nil {
		return nil, err
	}

	cgp := ngroupsclient.NewNGroupsClient(subID, pl)

	logger.Info("creating NGroup...")
	poller, err := cgp.BeginCreateOrUpdate(ctx, resourceGroupName, *nGroup.Name, *nGroup, nil)
	if err != nil {
		return nil, err
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, err
	}
	logger.Info("completed scaling out containers...")

	return map[string]string{}, nil
}

func (handler *azureCGNGroupsHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	subID := options.Resource.ID.FindScope("subscriptions")
	resourceGroupName := options.Resource.ID.FindScope("resourceGroups")

	pl, err := armruntime.NewPipeline("github.com/radius-project/radius", "v0.0.1", handler.arm.ClientOptions.Cred, azruntime.PipelineOptions{}, &armpolicy.ClientOptions{})
	if err != nil {
		return err
	}

	cgp := ngroupsclient.NewNGroupsClient(subID, pl)

	logger.Info("deleting NGroup...")
	poller, err := cgp.BeginDelete(ctx, resourceGroupName, options.Resource.ID.Name(), nil)
	if err != nil {
		return err
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return err
	}

	logger.Info("completed deleting containers...")
	return nil
}
