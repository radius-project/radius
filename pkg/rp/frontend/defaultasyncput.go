// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package defaultoperation

import (
	"context"
	"net/http"
	"time"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/rp"
)

var (
	// defaultAsyncPutTimeout is the default timeout duration of async put operation.
	defaultAsyncPutTimeout = time.Duration(120) * time.Second
)

// DefaultAsyncPut is the controller implementation to create or update async resource.
type DefaultAsyncPut[P interface {
	*T
	rp.RadiusResourceModel
}, T any] struct {
	ctrl.Operation[P, T]
}

// NewDefaultAsyncPut creates a new DefaultAsyncPut.
func NewDefaultAsyncPut[P interface {
	*T
	rp.RadiusResourceModel
}, T any](opts ctrl.Options, reqconv conv.ConvertToDataModel[T], respconv conv.ConvertToAPIModel[T]) (ctrl.Controller, error) {
	return &DefaultAsyncPut[P, T]{ctrl.NewOperation[P](opts, reqconv, respconv)}, nil
}

// Run executes DefaultAsyncPut operation.
func (e *DefaultAsyncPut[P, T]) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	newResource, err := e.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	old, etag, err := e.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if r, err := e.PrepareResource(ctx, req, newResource, old, etag); r != nil || err != nil {
		return r, err
	}

	if old != nil {
		oldProp := P(old).ResourceMetadata()
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
	}

	if r, err := e.PrepareAsyncOperation(ctx, newResource, v1.ProvisioningStateAccepted, defaultAsyncPutTimeout, &etag); r != nil || err != nil {
		return r, err
	}

	return e.ConstructAsyncResponse(ctx, req.Method, etag, newResource)
}
