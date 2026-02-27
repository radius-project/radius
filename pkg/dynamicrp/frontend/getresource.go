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
	"net/http"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/schema"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// GetResourceWithRedaction is a custom GET controller that redacts sensitive fields.
type GetResourceWithRedaction struct {
	ctrl.Operation[*datamodel.DynamicResource, datamodel.DynamicResource]
	ucpClient *v20231001preview.ClientFactory
}

// NewGetResourceWithRedaction creates a new GetResourceWithRedaction controller.
func NewGetResourceWithRedaction(
	opts ctrl.Options,
	resourceOpts ctrl.ResourceOptions[datamodel.DynamicResource],
	ucpClient *v20231001preview.ClientFactory,
) (ctrl.Controller, error) {
	return &GetResourceWithRedaction{
		Operation: ctrl.NewOperation[*datamodel.DynamicResource](opts, resourceOpts),
		ucpClient: ucpClient,
	}, nil
}

// Run returns the requested resource with sensitive fields redacted.
//
// Design consideration (GET Operation Update): When provisioningState is "Succeeded",
// the backend has already redacted sensitive data from the database, so we skip the
// schema fetch and redaction (fast path). For all other states (e.g., "Updating",
// "Accepted", "Failed"), the resource may still contain encrypted data, so we fetch
// the schema and redact sensitive fields to prevent exposure.
func (c *GetResourceWithRedaction) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	logger := ucplog.FromContextOrDiscard(ctx)

	resource, etag, err := c.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}
	if resource == nil {
		return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}

	// Fast path: if provisioningState is Succeeded, the backend has already redacted
	// sensitive fields. Skip the schema fetch for better performance.
	provisioningState := resource.ProvisioningState()
	if provisioningState != v1.ProvisioningStateSucceeded && resource.Properties != nil {
		resourceID := serviceCtx.ResourceID.String()
		resourceType := serviceCtx.ResourceID.Type()

		// Use the API version the resource was last updated with to ensure
		// encryption and redaction use the same schema
		apiVersion := resource.InternalMetadata.UpdatedAPIVersion

		sensitiveFieldPaths, err := schema.GetSensitiveFieldPaths(
			ctx,
			c.ucpClient,
			resourceID,
			resourceType,
			apiVersion,
		)
		if err != nil {
			logger.Error(err, "Failed to fetch sensitive field paths for GET redaction",
				"resourceType", resourceType, "apiVersion", apiVersion)
			// Fail-safe: return error to prevent potential exposure of sensitive data
			// This is consistent with the write path (encryption filter)
			return rest.NewInternalServerErrorARMResponse(v1.ErrorResponse{
				Error: &v1.ErrorDetails{
					Code:    v1.CodeInternal,
					Message: "Failed to fetch schema for security validation",
				},
			}), nil
		}

		if len(sensitiveFieldPaths) > 0 {
			schema.RedactFields(resource.Properties, sensitiveFieldPaths)
			logger.V(ucplog.LevelDebug).Info("Redacted sensitive fields in GET response",
				"provisioningState", provisioningState,
				"count", len(sensitiveFieldPaths), "resourceType", resourceType)
		}
	}

	return c.ConstructSyncResponse(ctx, req.Method, etag, resource)
}
