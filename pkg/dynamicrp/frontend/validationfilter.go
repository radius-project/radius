/*
Copyright 2026 The Radius Authors.

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
	"fmt"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/schema"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

type validationUpdateFilter controller.UpdateFilter[datamodel.DynamicResource]

func makeValidationFilter(ucpClient *v20231001preview.ClientFactory) validationUpdateFilter {
	return func(
		ctx context.Context,
		newResource *datamodel.DynamicResource,
		oldResource *datamodel.DynamicResource,
		options *controller.Options,
	) (rest.Response, error) {
		if ucpClient == nil {
			return nil, fmt.Errorf("UCP client is not configured for request validation")
		}
		serviceCtx := v1.ARMRequestContextFromContext(ctx)
		schemaData, err := schema.GetSchema(ctx, ucpClient, serviceCtx.ResourceID.String(), serviceCtx.ResourceID.Type(), serviceCtx.APIVersion)
		if err != nil {
			ucplog.FromContextOrDiscard(ctx).Error(err, "Failed to fetch schema for request validation",
				"resourceType", serviceCtx.ResourceID.Type(), "apiVersion", serviceCtx.APIVersion)
			return rest.NewInternalServerErrorARMResponse(v1.ErrorResponse{
				Error: &v1.ErrorDetails{
					Code:    v1.CodeInternal,
					Message: "Failed to fetch schema for request validation",
				},
			}), nil
		}
		if schemaData == nil {
			return nil, nil
		}

		resourceData := map[string]any{"properties": newResource.Properties}
		if err := schema.ValidateResourceRequestAgainstSchema(ctx, resourceData, schemaData); err != nil {
			return rest.NewBadRequestResponseWithCode(v1.CodeInvalidRequestContent,
				fmt.Sprintf("Schema validation failed: %v", err)), nil
		}
		return nil, nil
	}
}
