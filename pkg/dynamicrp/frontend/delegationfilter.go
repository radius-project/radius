// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package frontend

import (
	"context"
	"maps"
	"os"

	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/recipes/delegation"
	"github.com/radius-project/radius/pkg/schema"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
)

func makeDelegationStateFilter(ucp *v20231001preview.ClientFactory) validationUpdateFilter {
	return func(ctx context.Context, next, old *datamodel.DynamicResource, options *controller.Options) (rest.Response, error) {
		if os.Getenv(delegation.EnabledVariable) != "true" || old == nil {
			return nil, nil
		}
		resourceSchema, err := schema.GetSchema(ctx, ucp, next.ID, next.Type, next.UpdatedAPIVersion)
		if err != nil {
			return nil, err
		}
		if next.Properties == nil {
			next.Properties = map[string]any{}
		}
		if fields, ok := resourceSchema["properties"].(map[string]any); ok {
			for key, raw := range fields {
				field, _ := raw.(map[string]any)
				if readOnly, _ := field["readOnly"].(bool); readOnly {
					if value, exists := old.Properties[key]; exists {
						next.Properties[key] = value
					}
				}
			}
		}
		next.Properties["status"] = maps.Clone(old.Status())
		return nil, nil
	}
}
