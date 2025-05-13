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

package controller

import (
	"context"
	"fmt"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/recipes/engine"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// DynamicResourceController is the async operation controller to perform processing on dynamic resources.
//
// This controller will use the capabilities and the operation to determine the correct controller to use.
type DynamicResourceController struct {
	ctrl.BaseController

	ucp                 *v20231001preview.ClientFactory
	engine              engine.Engine
	configurationLoader configloader.ConfigurationLoader
}

// NewDynamicResourceController creates a new DynamicResourcePutController.
func NewDynamicResourceController(opts ctrl.Options, ucp *v20231001preview.ClientFactory, engine engine.Engine, configurationLoader configloader.ConfigurationLoader) (ctrl.Controller, error) {
	return &DynamicResourceController{
		BaseController:      ctrl.NewBaseAsyncController(opts),
		ucp:                 ucp,
		engine:              engine,
		configurationLoader: configurationLoader,
	}, nil
}

// Run implements the async controller interface.
func (c *DynamicResourceController) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	// This is where we have the opportunity to branch out to different controllers based on:
	// - The operation type. (eg: PUT, DELETE, etc)
	// - The capabilities of the resource type. (eg: Does it support recipes?)
	controller, err := c.selectController(ctx, request)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create controller: %w", err)
	}

	return controller.Run(ctx, request)

}

func (c *DynamicResourceController) selectController(ctx context.Context, request *ctrl.Request) (ctrl.Controller, error) {
	ot, ok := v1.ParseOperationType(request.OperationType)
	if !ok {
		return nil, fmt.Errorf("invalid operation type: %q", request.OperationType)
	}

	id, err := resources.ParseResource(request.ResourceID)
	if err != nil {
		return nil, fmt.Errorf("invalid resource ID: %q", request.ResourceID)
	}

	resourceType, err := c.fetchResourceTypeDetails(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch resource type for ID %q: %w", id.String(), err)
	}

	options := ctrl.Options{
		DatabaseClient: c.DatabaseClient(),
		ResourceType:   id.Type(),
		UcpClient:      c.ucp,
	}

	switch ot.Method {
	case v1.OperationDelete:
		if hasCapability(resourceType, datamodel.CapabilitySupportsRecipes) {
			return NewRecipeDeleteController(options, c.engine, c.configurationLoader)
		}
		return NewInertDeleteController(options)

	case v1.OperationPut:
		if hasCapability(resourceType, datamodel.CapabilitySupportsRecipes) {
			return NewRecipePutController(options, c.engine, c.configurationLoader)
		}
		return NewInertPutController(options)

	default:
		return nil, fmt.Errorf("unsupported operation type: %q", request.OperationType)
	}
}

// fetchResourceTypeDetails fetches the resource type details from the UCP API for the given resource ID.
func (c *DynamicResourceController) fetchResourceTypeDetails(ctx context.Context, id resources.ID) (*v20231001preview.ResourceTypeResource, error) {
	providerNamespace := id.ProviderNamespace()
	planeName := id.ScopeSegments()[0].Name
	resourceTypeName := strings.TrimPrefix(id.Type(), providerNamespace+resources.SegmentSeparator)
	response, err := c.ucp.NewResourceTypesClient().Get(
		ctx,
		planeName,
		providerNamespace,
		resourceTypeName,
		nil)
	if err != nil {
		return nil, err
	}

	return &response.ResourceTypeResource, nil
}

// hasCapability determines if a resource type has a specific capability.
// It returns true when the given input capability string exists in the resource type's
// capabilities list, false otherwise.
func hasCapability(resourceType *v20231001preview.ResourceTypeResource, capability string) bool {
	for _, c := range resourceType.Properties.Capabilities {
		if c != nil && *c == capability {
			return true
		}
	}

	return false
}
