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
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

func NewAzureCGScaleSetHandler(arm *armauth.ArmConfig) ResourceHandler {
	return &azureCGScaleSetHandler{arm: arm}
}

type azureCGScaleSetHandler struct {
	arm *armauth.ArmConfig
}

func (handler *azureCGScaleSetHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	cs2, ok := options.Resource.CreateResource.Data.(*cs2client.ContainerScaleSet)
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

	cs2.Properties.ContainerGroupProfiles[0].Resource.ID = to.Ptr(cgpID)
	cgp, err := cs2client.NewContainerScaleSetsClient(subID, handler.arm.ClientOptions.Cred, nil)
	if err != nil {
		return nil, err
	}

	logger.Info("creating container scale set...")
	poller, err := cgp.BeginCreateOrUpdate(ctx, resourceGroupName, *cs2.Name, *cs2, nil)
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

func (handler *azureCGScaleSetHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	subID := options.Resource.ID.FindScope("subscriptions")
	resourceGroupName := options.Resource.ID.FindScope("resourceGroups")

	cgp, err := cs2client.NewContainerScaleSetsClient(subID, handler.arm.ClientOptions.Cred, nil)
	if err != nil {
		return err
	}

	logger.Info("deleting container scale set...")
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
