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
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
)

const statusProperty = "status"

type statusUpdateFilter controller.UpdateFilter[datamodel.DynamicResource]

// makeStatusFilter keeps properties.status owned by the server.
//
// A PUT replaces the stored resource before the backend runs. The backend reads the output resources
// of the previous deployment from properties.status, so an update must carry the stored status
// forward instead of the request's. Built-in portable resources do the same in PrepareRadiusResource.
// It runs after the defaults filter, so the scope check sees a defaulted environment or application.
func makeStatusFilter() statusUpdateFilter {
	return func(
		ctx context.Context,
		newResource *datamodel.DynamicResource,
		oldResource *datamodel.DynamicResource,
		options *controller.Options,
	) (rest.Response, error) {
		delete(newResource.Properties, statusProperty)

		if oldResource == nil {
			return nil, nil
		}

		status, ok := oldResource.Properties[statusProperty]
		if !ok {
			return nil, nil
		}

		// Output resources are deleted using the environment and application they were deployed with.
		// Moving a resource that has them would delete or orphan them with the wrong configuration.
		oldScope, newScope := oldResource.ResourceMetadata(), newResource.ResourceMetadata()
		if len(oldResource.OutputResources()) > 0 && !rpv1.ScopesEqual(oldScope, newScope) {
			serviceCtx := v1.ARMRequestContextFromContext(ctx)
			return rest.NewLinkedResourceUpdateErrorResponse(serviceCtx.ResourceID, oldScope, newScope), nil
		}

		if newResource.Properties == nil {
			newResource.Properties = map[string]any{}
		}
		newResource.Properties[statusProperty] = status

		return nil, nil
	}
}
