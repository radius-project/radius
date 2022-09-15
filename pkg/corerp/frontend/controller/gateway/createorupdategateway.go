// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package gateway

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

var _ ctrl.Controller = (*CreateOrUpdateGateway)(nil)

// AsyncPutGatewayOperationTimeout is the default timeout duration of async put gateway operation.
var AsyncPutGatewayOperationTimeout = time.Duration(120) * time.Second

// CreateOrUpdateGateway is the controller implementation to create or update a gateway resource.
type CreateOrUpdateGateway struct {
	ctrl.Operation[*rm, rm]
}

// NewCreateOrUpdateGateway creates a new CreateOrUpdateGateway.
func NewCreateOrUpdateGateway(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateGateway{
		ctrl.NewOperation(opts, converter.GatewayDataModelFromVersioned, converter.GatewayDataModelToVersioned),
	}, nil
}

// Run executes CreateOrUpdateGateway operation.
func (e *CreateOrUpdateGateway) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	newResource, err := e.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	old, etag, err := e.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if err := e.ValidateResource(ctx, req, newResource, old, etag); err != nil {
		return nil, err
	}

	if old == nil {
		newResource.UpdateMetadata(serviceCtx, nil)
	} else {
		newResource.UpdateMetadata(serviceCtx, &old.SystemData)
		if !old.Properties.ProvisioningState.IsTerminal() {
			return rest.NewConflictResponse(fmt.Sprintf(ctrl.InProgressStateMessageFormat, old.Properties.ProvisioningState)), nil
		}

		if err := e.ValidateLinkedResource(serviceCtx.ResourceID, &newResource.Properties.BasicResourceProperties, &old.Properties.BasicResourceProperties); err != nil {
			return nil, err
		}

		// Gateway is a resource that is asyncly processed. Here, in createOrUpdateGateway, we
		// don't do the rendering and the deployment. newResource is collected from the request and
		// that is why newResource doesn't have outputResources. It is wiped in the save call 2
		// lines below. Because we are saving newResource and newResource doesn't have the output
		// resources array. When we don't know the outputResources of a resource, we can't delete
		// the ones that are not needed when we are deploying a new version of that resource.
		// Gateway X - v1 => OutputResources[Y,Z]
		// During the createOrUpdateGateway call Gateway X loses the OutputResources array
		// because it is wiped from the DB when we are saving the newResource.
		// Gateway X - v2 needs to be deployed and because we don't know the outputResources
		// of v1, we don't know which one to delete.
		newResource.Properties.Status.DeepCopy(&old.Properties.Status)
	}

	newResource.Properties.ProvisioningState = v1.ProvisioningStateAccepted

	newEtag, err := e.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	if err := e.StatusManager().QueueAsyncOperation(ctx, serviceCtx, AsyncPutGatewayOperationTimeout); err != nil {
		newResource.Properties.ProvisioningState = v1.ProvisioningStateFailed
		_, rbErr := e.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, newEtag)
		if rbErr != nil {
			return nil, rbErr
		}
		return nil, err
	}

	return e.ConstructAsyncResponse(ctx, req.Method, etag, newResource)
}
