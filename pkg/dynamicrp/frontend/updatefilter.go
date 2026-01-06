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

	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/dynamicrp/schema"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// UpdateFilterFactory creates an UpdateFilter that has access to the UCP client for schema lookups.
type UpdateFilterFactory struct {
	UCPClient *v20231001preview.ClientFactory
}

// NewPrepareResourceFilter returns an UpdateFilter function that can access the UCP client.
func (f *UpdateFilterFactory) NewPrepareResourceFilter() controller.UpdateFilter[datamodel.DynamicResource] {
	return func(ctx context.Context, newResource, oldResource *datamodel.DynamicResource, opt *controller.Options) (rest.Response, error) {
		return f.prepareResource(ctx, newResource, oldResource, opt)
	}
}

// prepareResource extracts sensitive field paths from the schema during PUT/PATCH operations.
// The backend will independently call the same schema function when it needs the paths.
func (f *UpdateFilterFactory) prepareResource(ctx context.Context, newResource, _ *datamodel.DynamicResource, _ *controller.Options) (rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info("PrepareResource filter executing for dynamic resource", "resourceID", newResource.ID)

	// Extract sensitive field paths from the schema
	// This is called here in the frontend; the backend will call the same function independently
	sensitiveFieldPaths, err := schema.GetSensitiveFieldPaths(
		ctx,
		f.UCPClient,
		newResource.ID,
		newResource.Type,
		newResource.InternalMetadata.UpdatedAPIVersion,
	)
	if err != nil {
		logger.Error(err, "Failed to get sensitive field paths from schema", "resourceID", newResource.ID)
		// Continue processing even if we can't get the schema - don't fail the request
	}

	if len(sensitiveFieldPaths) > 0 {
		logger.Info("Found sensitive fields in schema", "resourceID", newResource.ID, "paths", sensitiveFieldPaths)
	}

	// Note: We intentionally do NOT store the paths on the resource.
	// The backend will call schema.GetSensitiveFieldPaths() independently when needed.

	// TODO: We need to update this to return or save the data from sensitive fields once we make changes to the frontend code.
	return nil, nil
}
