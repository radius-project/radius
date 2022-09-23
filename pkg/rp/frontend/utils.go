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

	// T is a resource that is asyncly processed. Here, we
	// don't do the rendering and the deployment. newResource is collected from the request and
	// that is why newResource doesn't have outputResources. It is wiped in the save call 2
	// lines below. Because we are saving newResource and newResource doesn't have the output
	// resources array. When we don't know the outputResources of a resource, we can't delete
	// the ones that are not needed when we are deploying a new version of that resource.
	// T X - v1 => OutputResources[Y,Z]
	// During the createOrUpdateHttpRoute call HttpRoute X loses the OutputResources array
	// because it is wiped from the DB when we are saving the newResource.
	// T X - v2 needs to be deployed and because we don't know the outputResources
	// of v1, we don't know which one to delete.
	newProp.Status.DeepCopy(&oldProp.Status)

	return nil, nil
}
