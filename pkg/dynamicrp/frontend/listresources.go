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
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/schema"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// ListResourcesWithRedaction is a custom LIST controller that redacts sensitive fields.
type ListResourcesWithRedaction struct {
	ctrl.Operation[*datamodel.DynamicResource, datamodel.DynamicResource]
	ucpClient          *v20231001preview.ClientFactory
	listRecursiveQuery bool
}

// NewListResourcesWithRedaction creates a new ListResourcesWithRedaction controller.
func NewListResourcesWithRedaction(
	opts ctrl.Options,
	resourceOpts ctrl.ResourceOptions[datamodel.DynamicResource],
	ucpClient *v20231001preview.ClientFactory,
) (ctrl.Controller, error) {
	return &ListResourcesWithRedaction{
		Operation:          ctrl.NewOperation[*datamodel.DynamicResource](opts, resourceOpts),
		ucpClient:          ucpClient,
		listRecursiveQuery: resourceOpts.ListRecursiveQuery,
	}, nil
}

// Run returns the list of resources with sensitive fields redacted.
func (c *ListResourcesWithRedaction) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	logger := ucplog.FromContextOrDiscard(ctx)

	query := database.Query{
		RootScope:      serviceCtx.ResourceID.RootScope(),
		ResourceType:   serviceCtx.ResourceID.Type(),
		ScopeRecursive: c.listRecursiveQuery,
	}

	result, err := c.DatabaseClient().Query(ctx, query, database.WithPaginationToken(serviceCtx.SkipToken), database.WithMaxQueryItemCount(serviceCtx.Top))
	if err != nil {
		return nil, err
	}

	// Cache sensitive field paths for this resource type (same for all items)
	var sensitiveFieldPaths []string
	var sensitiveFieldPathsFetched bool

	items := []any{}
	for _, item := range result.Items {
		resource := &datamodel.DynamicResource{}
		if err := item.As(resource); err != nil {
			return nil, err
		}

		// Redact sensitive fields before adding to the response.
		// Fast path: if provisioningState is Succeeded, the backend has already redacted
		// sensitive fields. Skip redaction for these items.
		provisioningState := resource.ProvisioningState()
		if provisioningState != v1.ProvisioningStateSucceeded && resource.Properties != nil {
			// Fetch sensitive field paths once for this resource type
			if !sensitiveFieldPathsFetched {
				apiVersion := getResourceAPIVersion(serviceCtx.APIVersion, resource)
				sensitiveFieldPaths, err = schema.GetSensitiveFieldPaths(
					ctx,
					c.ucpClient,
					resource.ID,
					serviceCtx.ResourceID.Type(),
					apiVersion,
				)
				if err != nil {
					fallbackAPIVersion := getResourceAPIVersion("", resource)
					if fallbackAPIVersion != "" && fallbackAPIVersion != apiVersion {
						sensitiveFieldPaths, err = schema.GetSensitiveFieldPaths(
							ctx,
							c.ucpClient,
							resource.ID,
							serviceCtx.ResourceID.Type(),
							fallbackAPIVersion,
						)
						apiVersion = fallbackAPIVersion
					}
				}
				if err != nil {
					logger.Error(err, "Failed to fetch sensitive field paths for LIST redaction",
						"resourceType", serviceCtx.ResourceID.Type(), "apiVersion", apiVersion)
					// Continue without redaction on error
				}
				sensitiveFieldPathsFetched = true
			}

			if len(sensitiveFieldPaths) > 0 {
				for _, path := range sensitiveFieldPaths {
					redactField(resource.Properties, path)
				}
			}
		}

		versioned, err := c.ResponseConverter()(resource, serviceCtx.APIVersion)
		if err != nil {
			return nil, err
		}
		items = append(items, versioned)
	}

	if len(sensitiveFieldPaths) > 0 {
		logger.V(ucplog.LevelDebug).Info("Redacted sensitive fields in LIST response",
			"count", len(sensitiveFieldPaths), "resourceType", serviceCtx.ResourceID.Type(),
			"itemCount", len(items))
	}

	return rest.NewOKResponse(&v1.PaginatedList{
		Value:    items,
		NextLink: ctrl.GetNextLinkURL(ctx, req, result.PaginationToken),
	}), nil
}
