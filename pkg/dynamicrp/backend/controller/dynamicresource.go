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
	"errors"
	"fmt"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/dynamicrp/backend/processor"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/recipes/engine"
	"github.com/radius-project/radius/pkg/schema"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
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

// extractOperationAndResourceType parses the operation type and fetches resource type details.
// Returns the parsed operation type, resource type details from UCP, and any error encountered.
// This function is shared by both selectController and validateRequestSchema for consistency.
func (c *DynamicResourceController) extractOperationAndResourceType(ctx context.Context, request *ctrl.Request) (v1.OperationType, *v20231001preview.ResourceTypeResource, error) {
	parsedOperationType, ok := v1.ParseOperationType(request.OperationType)
	if !ok {
		return v1.OperationType{}, nil, fmt.Errorf("invalid operation type: %q", request.OperationType)
	}

	id, err := resources.ParseResource(request.ResourceID)
	if err != nil {
		return v1.OperationType{}, nil, fmt.Errorf("invalid resource ID: %q", request.ResourceID)
	}

	resourceTypeDetails, err := c.fetchResourceTypeDetails(ctx, id)
	if err != nil {
		return v1.OperationType{}, nil, fmt.Errorf("failed to fetch resource type details: %w", err)
	}

	return parsedOperationType, resourceTypeDetails, nil
}

// Run implements the async controller interface.
func (c *DynamicResourceController) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	// Validate request body against schema if available
	if err := c.validateRequestSchema(ctx, request); err != nil {
		return ctrl.Result{}, err
	}

	// This is where we have the opportunity to branch out to different controllers based on:
	// - The operation type. (eg: PUT, DELETE, etc)
	// - The capabilities of the resource type. (eg: Does it support recipes?)
	controller, err := c.selectController(ctx, request)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create controller: %w", err)
	}

	return controller.Run(ctx, request)
}

// validateRequestSchema validates the request body against the resource type's schema
func (c *DynamicResourceController) validateRequestSchema(ctx context.Context, request *ctrl.Request) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Extract operation context once
	operationContext, resourceTypeDetails, err := c.extractOperationAndResourceType(ctx, request)
	if err != nil {
		return err
	}

	// Skip validation for non-PUT operations
	if operationContext.Method != v1.OperationPut {
		return nil
	}

	// Get the resource data from storage (for existing resources)
	resourceData, err := c.getResourceDataFromStorage(ctx, request.ResourceID)
	if err != nil {
		// Unexpected error accessing storage
		return fmt.Errorf("failed to access and validate resource data: %w", err)
	}

	if resourceData == nil {
		// New resource - no existing data to validate against
		logger := ucplog.FromContextOrDiscard(ctx)
		logger.V(ucplog.LevelDebug).Info("No existing resource data found, skipping validation for new resource", "resourceID", request.ResourceID)
		return nil
	}

	logger.Info("Validating existing resource data against schema", "resourceID", request.ResourceID)

	// Use the API version from the request
	apiVersion := request.APIVersion

	// Get the schema for the resource type
	schemaData, err := processor.GetSchemaForResourceType(ctx, c.ucp, *resourceTypeDetails.ID, apiVersion)
	if err != nil {
		if errors.Is(err, processor.ErrNoSchemaFound) {
			logger := ucplog.FromContextOrDiscard(ctx)
			logger.V(ucplog.LevelDebug).Info("No schema found for resource type, skipping validation", "resourceType", *resourceTypeDetails.Name)
			return nil
		}
		return fmt.Errorf("failed to get schema: %w", err)
	}

	// Validate the resource against the schema using the schema package
	if err := schema.ValidateResourceAgainstSchema(ctx, resourceData, schemaData); err != nil {
		return fmt.Errorf("schema validation failed: %w", err)
	}

	return nil
}

// selectController determines which controller to use based on the operation and resource capabilities
func (c *DynamicResourceController) selectController(ctx context.Context, request *ctrl.Request) (ctrl.Controller, error) {
	operationType, resourceTypeDetails, err := c.extractOperationAndResourceType(ctx, request)
	if err != nil {
		return nil, err
	}

	options := ctrl.Options{
		DatabaseClient: c.DatabaseClient(),
		ResourceType:   *resourceTypeDetails.Name,
		UcpClient:      c.ucp,
	}

	switch operationType.Method {
	case v1.OperationDelete:
		if hasCapability(resourceType, datamodel.CapabilityManualResourceProvisioning) {
			return NewInertDeleteController(options)
		}
		return NewRecipeDeleteController(options, c.engine, c.configurationLoader)

	case v1.OperationPut:
		if hasCapability(resourceType, datamodel.CapabilityManualResourceProvisioning) {
			return NewInertPutController(options)
		}
		return NewRecipePutController(options, c.engine, c.configurationLoader)

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

// getResourceDataFromStorage retrieves resource data from storage and converts it to a map
func (c *DynamicResourceController) getResourceDataFromStorage(ctx context.Context, resourceID string) (map[string]any, error) {
	storageClient := c.DatabaseClient()
	obj, err := storageClient.Get(ctx, resourceID)
	if err != nil {
		return nil, err
	}

	// Extract the resource data for validation
	resourceData := obj.Data
	if resourceData == nil {
		return nil, nil
	}

	// Convert resource data to map for validation
	var resourceMap map[string]any
	resourceMap, ok := resourceData.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("resource data is not a valid map[string]any")
	}

	return resourceMap, nil
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
