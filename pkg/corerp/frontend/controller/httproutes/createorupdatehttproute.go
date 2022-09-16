// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package httproutes

import (
	"context"
	"fmt"
	"net/http"
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
)

var (
	_ ctrl.Controller = (*CreateOrUpdateHTTPRoute)(nil)

	// AsyncPutHTTPRouteOperationTimeout is the default timeout duration of async put httproute operation.
	AsyncPutHTTPRouteOperationTimeout = time.Duration(120) * time.Second
)

// CreateOrUpdateHTTPRoute is the controller implementation to create or update HTTPRoute resource.
type CreateOrUpdateHTTPRoute struct {
	ctrl.Operation[*rm, rm]
}

// NewCreateOrUpdateTTPRoute creates a new CreateOrUpdateHTTPRoute.
func NewCreateOrUpdateHTTPRoute(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateHTTPRoute{
		ctrl.NewOperation(opts, converter.HTTPRouteDataModelFromVersioned, converter.HTTPRouteDataModelToVersioned),
	}, nil
}

// Run executes CreateOrUpdateHTTPRoute operation.
func (e *CreateOrUpdateHTTPRoute) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	newResource, err := e.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	old, etag, err := e.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if r := e.ValidateResource(ctx, req, newResource, old, etag); r != nil {
		return r, nil
	}

	if old == nil {
		newResource.UpdateMetadata(serviceCtx, nil)
	} else {
		newResource.UpdateMetadata(serviceCtx, &old.SystemData)
		if !old.Properties.ProvisioningState.IsTerminal() {
			return rest.NewConflictResponse(fmt.Sprintf(ctrl.InProgressStateMessageFormat, old.Properties.ProvisioningState)), nil
		}

		oldProp := &old.Properties.BasicResourceProperties
		newProp := &newResource.Properties.BasicResourceProperties
		if !oldProp.EqualLinkedResource(newProp) {
			return rest.NewLinkedResourceUpdateErrorResponse(serviceCtx.ResourceID, oldProp, newProp), nil
		}

		// HttpRoute is a resource that is asyncly processed. Here, in createOrUpdateHttpRoute, we
		// don't do the rendering and the deployment. newResource is collected from the request and
		// that is why newResource doesn't have outputResources. It is wiped in the save call 2
		// lines below. Because we are saving newResource and newResource doesn't have the output
		// resources array. When we don't know the outputResources of a resource, we can't delete
		// the ones that are not needed when we are deploying a new version of that resource.
		// HttpRoute X - v1 => OutputResources[Y,Z]
		// During the createOrUpdateHttpRoute call HttpRoute X loses the OutputResources array
		// because it is wiped from the DB when we are saving the newResource.
		// HttpRoute X - v2 needs to be deployed and because we don't know the outputResources
		// of v1, we don't know which one to delete.
		newResource.Properties.Status.DeepCopy(&old.Properties.Status)
	}

	newResource.Properties.ProvisioningState = v1.ProvisioningStateAccepted

	newETag, err := e.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	if err := e.StatusManager().QueueAsyncOperation(ctx, serviceCtx, AsyncPutHTTPRouteOperationTimeout); err != nil {
		newResource.Properties.ProvisioningState = v1.ProvisioningStateFailed
		_, rbErr := e.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, newETag)
		if rbErr != nil {
			return nil, rbErr
		}
		return nil, err
	}

	return e.ConstructAsyncResponse(ctx, req.Method, etag, newResource)
}
