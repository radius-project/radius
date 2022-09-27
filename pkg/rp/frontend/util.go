// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package frontend

import (
	"context"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/rp"
)

// PrepareRadiusResource validates the Radius resource and prepare new resource data.
func PrepareRadiusResource[P interface {
	*T
	rp.RadiusResourceModel
}, T any](ctx context.Context, oldResource *T, newResource *T) (rest.Response, error) {
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
