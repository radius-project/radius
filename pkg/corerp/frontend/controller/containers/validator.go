// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containers

import (
	"context"

	"github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
)

// ValidateRequest validates and mutate the incoming request.
func ValidateAndMutateRequest(ctx context.Context, newResource *datamodel.ContainerResource, oldResource *datamodel.ContainerResource, options *controller.Options) (rest.Response, error) {
	if oldResource != nil {
		// Populate the existing identity.
		newResource.Properties.Identity = oldResource.Properties.Identity
	}

	return nil, nil
}
