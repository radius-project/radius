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

package frontend

import (
	"context"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/schema"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// makeDefaultsFilter creates an UpdateFilter that materializes schema defaults into the resource's
// Properties before it is saved, so the stored resource, the API response and the recipe agree.
//
// oldResource is deliberately not consulted: a PUT replaces the resource, so a dropped property
// returns to its default rather than keeping its previous value.
func makeDefaultsFilter(ucpClient *v20231001preview.ClientFactory) controller.UpdateFilter[datamodel.DynamicResource] {
	return func(
		ctx context.Context,
		newResource *datamodel.DynamicResource,
		oldResource *datamodel.DynamicResource,
		options *controller.Options,
	) (rest.Response, error) {
		return applySchemaDefaults(ctx, newResource, ucpClient)
	}
}

// applySchemaDefaults fills unset properties from the "default" values declared in the resource type schema.
func applySchemaDefaults(
	ctx context.Context,
	newResource *datamodel.DynamicResource,
	ucpClient *v20231001preview.ClientFactory,
) (rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	if newResource == nil {
		return nil, nil
	}

	resourceID := serviceCtx.ResourceID.String()
	resourceType := serviceCtx.ResourceID.Type()
	apiVersion := serviceCtx.APIVersion

	schemaData, err := schema.GetSchema(ctx, ucpClient, resourceID, resourceType, apiVersion)
	if err != nil {
		logger.Error(err, "Failed to fetch schema for defaults",
			"resourceType", resourceType, "apiVersion", apiVersion)
		return rest.NewInternalServerErrorARMResponse(v1.ErrorResponse{
			Error: &v1.ErrorDetails{
				Code:    v1.CodeInternal,
				Message: "Failed to fetch schema to apply property defaults",
			},
		}), nil
	}

	if schemaData == nil {
		return nil, nil
	}

	// Attach the map only if something was applied, so an empty resource serializes as before.
	properties := newResource.Properties
	if properties == nil {
		properties = map[string]any{}
	}

	if applied := schema.ApplyDefaults(properties, schemaData); applied > 0 {
		newResource.Properties = properties
		logger.V(ucplog.LevelDebug).Info("Applied schema defaults",
			"count", applied, "resourceType", resourceType, "resourceID", resourceID)
	}

	return nil, nil
}
