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
	"strings"

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
		apiVersion := getResourceAPIVersion(serviceCtx.APIVersion, resource)

		sensitiveFieldPaths, err := schema.GetSensitiveFieldPaths(
			ctx,
			c.ucpClient,
			resourceID,
			resourceType,
			apiVersion,
		)
		if err != nil {
			fallbackAPIVersion := getResourceAPIVersion("", resource)
			if fallbackAPIVersion != "" && fallbackAPIVersion != apiVersion {
				sensitiveFieldPaths, err = schema.GetSensitiveFieldPaths(
					ctx,
					c.ucpClient,
					resourceID,
					resourceType,
					fallbackAPIVersion,
				)
				apiVersion = fallbackAPIVersion
			}
		}
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
				"provisioningState", provisioningState,
				"count", len(sensitiveFieldPaths), "resourceType", resourceType)
		}
	}

	return c.ConstructSyncResponse(ctx, req.Method, etag, resource)
}

func getResourceAPIVersion(requestAPIVersion string, resource *datamodel.DynamicResource) string {
	if requestAPIVersion != "" {
		return requestAPIVersion
	}

	if resource == nil {
		return ""
	}

	metadata := resource.InternalMetadata
	if metadata.UpdatedAPIVersion != "" {
		return metadata.UpdatedAPIVersion
	}
	return metadata.CreatedAPIVersion
}

// redactField sets the field at the given path to nil.
// Supports:
//   - Simple field names: "data"
//   - Nested dot-separated paths: "credentials.password"
//   - Array wildcards: "secrets[*].value"
//   - Map wildcards: "config[*]"
func redactField(properties map[string]any, path string) {
	if properties == nil || path == "" {
		return
	}

	segments := parseRedactPath(path)
	if len(segments) == 0 {
		return
	}

	redactAtSegments(properties, segments)
}

// redactPathSegment represents a component of a field path for redaction.
type redactPathSegment struct {
	name     string // field name (empty for wildcard)
	wildcard bool   // true for [*] segments
}

// parseRedactPath parses a field path like "credentials.password" or "secrets[*].value"
// into path segments for traversal.
func parseRedactPath(path string) []redactPathSegment {
	var segments []redactPathSegment
	var current strings.Builder

	for i := 0; i < len(path); i++ {
		switch path[i] {
		case '.':
			if current.Len() > 0 {
				segments = append(segments, redactPathSegment{name: current.String()})
				current.Reset()
			}
		case '[':
			if current.Len() > 0 {
				segments = append(segments, redactPathSegment{name: current.String()})
				current.Reset()
			}
			end := strings.Index(path[i:], "]")
			if end == -1 {
				return nil // invalid path - unterminated bracket
			}
			content := path[i+1 : i+end]
			if content == "*" {
				segments = append(segments, redactPathSegment{wildcard: true})
			}
			i += end // skip past ']'
		default:
			current.WriteByte(path[i])
		}
	}

	if current.Len() > 0 {
		segments = append(segments, redactPathSegment{name: current.String()})
	}

	return segments
}

// redactAtSegments traverses the data following the path segments and sets the final value to nil.
func redactAtSegments(current any, segments []redactPathSegment) {
	if len(segments) == 0 {
		return
	}

	segment := segments[0]
	remaining := segments[1:]

	if segment.wildcard {
		// Handle [*] - iterate over array or map
		switch v := current.(type) {
		case []any:
			for i := range v {
				if len(remaining) == 0 {
					v[i] = nil
				} else {
					redactAtSegments(v[i], remaining)
				}
			}
		case map[string]any:
			for key := range v {
				if len(remaining) == 0 {
					v[key] = nil
				} else {
					redactAtSegments(v[key], remaining)
				}
			}
		}
		return
	}

	// Handle field name
	dataMap, ok := current.(map[string]any)
	if !ok {
		return
	}

	value, exists := dataMap[segment.name]
	if !exists {
		return
	}

	if len(remaining) == 0 {
		dataMap[segment.name] = nil
		return
	}

	redactAtSegments(value, remaining)
}
