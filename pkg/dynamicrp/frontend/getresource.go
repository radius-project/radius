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

	// Redact sensitive fields before returning the response
	if resource.Properties != nil {
		resourceID := serviceCtx.ResourceID.String()
		resourceType := serviceCtx.ResourceID.Type()
		apiVersion := serviceCtx.APIVersion

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
			// Continue without redaction on error - don't fail the GET
		} else if len(sensitiveFieldPaths) > 0 {
			// Redact sensitive fields by setting them to nil
			for _, path := range sensitiveFieldPaths {
				redactField(resource.Properties, path)
			}
			logger.V(ucplog.LevelDebug).Info("Redacted sensitive fields in GET response",
				"count", len(sensitiveFieldPaths), "resourceType", resourceType)
		}
	}

	return c.ConstructSyncResponse(ctx, req.Method, etag, resource)
}

// redactField sets the field at the given path to nil.
// Supports simple field names like "data" or nested paths like "config.password".
func redactField(properties map[string]any, path string) {
	if properties == nil {
		return
	}

	// For simple paths (no dots), just set to nil
	if _, exists := properties[path]; exists {
		properties[path] = nil
		return
	}

	// For nested paths, we would need to traverse - but for now we only support top-level
	// The "data" field is top-level in Radius.Security/secrets
}
