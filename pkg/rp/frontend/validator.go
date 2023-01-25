// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package frontend

import (
	"context"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// PrepareRadiusResource validates the Radius resource and prepare new resource data.
func PrepareRadiusResource[P interface {
	*T
	rpv1.RadiusResourceModel
}, T any](ctx context.Context, newResource *T, oldResource *T, options *controller.Options) (rest.Response, error) {
	if oldResource == nil {
		return nil, nil
	}
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	oldProp := P(oldResource).ResourceMetadata()
	newProp := P(newResource).ResourceMetadata()

	if !oldProp.EqualLinkedResource(newProp) {
		return rest.NewLinkedResourceUpdateErrorResponse(serviceCtx.ResourceID, oldProp, newProp), nil
	}

	// Keep outputresource from existing resource since the incoming request hasn't had an outputresource
	// processed by the backend yet.
	newProp.Status.DeepCopy(&oldProp.Status)

	return nil, nil
}
