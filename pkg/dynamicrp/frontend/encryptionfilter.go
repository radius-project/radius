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
	"github.com/radius-project/radius/pkg/crypto/encryption"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/schema"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// makeEncryptionFilter creates an UpdateFilter that encrypts sensitive fields in the resource's
// Properties map before saving to the database.
//
// The filter:
// 1. Fetches the schema for the resource type to identify sensitive field paths (marked with x-radius-sensitive)
// 2. Encrypts values at those paths using the SensitiveDataHandler
// 3. Uses the resource ID as associated data for context binding (prevents moving encrypted values between resources)
//
// If sensitive fields are not found, the resource passes through unchanged.
func makeEncryptionFilter(
	ucpClient *v20231001preview.ClientFactory,
	handler *encryption.SensitiveDataHandler,
) controller.UpdateFilter[datamodel.DynamicResource] {
	return func(
		ctx context.Context,
		newResource *datamodel.DynamicResource,
		oldResource *datamodel.DynamicResource,
		options *controller.Options,
	) (rest.Response, error) {
		return encryptSensitiveFields(ctx, newResource, ucpClient, handler)
	}
}

// encryptSensitiveFields encrypts fields marked with x-radius-sensitive annotation in the resource schema.
func encryptSensitiveFields(
	ctx context.Context,
	newResource *datamodel.DynamicResource,
	ucpClient *v20231001preview.ClientFactory,
	handler *encryption.SensitiveDataHandler,
) (rest.Response, error) {
	// No-op if handler is nil (encryption not configured)
	if handler == nil {
		return nil, nil
	}

	logger := ucplog.FromContextOrDiscard(ctx)
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	resourceID := serviceCtx.ResourceID.String()
	resourceType := serviceCtx.ResourceID.Type()
	apiVersion := serviceCtx.APIVersion

	// Fetch sensitive field paths from schema
	sensitiveFieldPaths, err := schema.GetSensitiveFieldPaths(
		ctx,
		ucpClient,
		resourceID,
		resourceType,
		apiVersion,
	)
	if err != nil {
		logger.Error(err, "Failed to fetch sensitive field paths",
			"resourceType", resourceType, "apiVersion", apiVersion)
		return rest.NewInternalServerErrorARMResponse(v1.ErrorResponse{
			Error: &v1.ErrorDetails{
				Code:    v1.CodeInternal,
				Message: "Failed to fetch schema for sensitive field encryption",
			},
		}), nil
	}

	// No sensitive fields to encrypt
	if len(sensitiveFieldPaths) == 0 {
		return nil, nil
	}

	// No properties to encrypt
	if newResource.Properties == nil {
		return nil, nil
	}

	// Encrypt sensitive fields in the Properties map
	// Field paths from schema are relative to "properties", so we operate on Properties directly
	if err := handler.EncryptSensitiveFields(
		newResource.Properties,
		sensitiveFieldPaths,
		resourceID,
	); err != nil {
		logger.Error(err, "Failed to encrypt sensitive fields",
			"resourceType", resourceType, "resourceID", resourceID)
		return rest.NewInternalServerErrorARMResponse(v1.ErrorResponse{
			Error: &v1.ErrorDetails{
				Code:    v1.CodeInternal,
				Message: "Failed to encrypt sensitive fields",
			},
		}), nil
	}

	logger.V(ucplog.LevelDebug).Info("Encrypted sensitive fields",
		"count", len(sensitiveFieldPaths), "resourceType", resourceType)

	return nil, nil
}
