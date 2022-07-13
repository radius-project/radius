// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package gateway

import (
	"context"
	"errors"
	"net/http"
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	"github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*CreateOrUpdateGateway)(nil)

// AsyncPutGatewayOperationTimeout is the default timeout duration of async put gateway operation.
var AsyncPutGatewayOperationTimeout = time.Duration(120) * time.Second

// CreateOrUpdateGateway is the controller implementation to create or update a gateway resource.
type CreateOrUpdateGateway struct {
	ctrl.BaseController
}

// NewCreateOrUpdateGateway creates a new CreateOrUpdateGateway.
func NewCreateOrUpdateGateway(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateGateway{ctrl.NewBaseController(opts)}, nil
}

// Run executes CreateOrUpdateGateway operation.
func (e *CreateOrUpdateGateway) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	newResource, err := e.Validate(ctx, req, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	existingResource := &datamodel.Gateway{}
	etag, err := e.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if err != nil && !errors.Is(&store.ErrNotFound{}, err) {
		return nil, err
	}

	exists := true
	if err != nil && errors.Is(&store.ErrNotFound{}, err) {
		exists = false
	}

	if req.Method == http.MethodPatch && !exists {
		return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}

	if exists && !existingResource.Properties.ProvisioningState.IsTerminal() {
		return rest.NewConflictResponse(controller.OngoingAsyncOperationOnResourceMessage), nil
	}

	err = ctrl.ValidateETag(*serviceCtx, etag)
	if err != nil {
		return rest.NewPreconditionFailedResponse(serviceCtx.ResourceID.String(), err.Error()), nil
	}

	enrichMetadata(serviceCtx, existingResource, newResource)

	obj, err := e.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	err = e.StatusManager().QueueAsyncOperation(ctx, serviceCtx, AsyncPutGatewayOperationTimeout)
	if err != nil {
		newResource.Properties.ProvisioningState = v1.ProvisioningStateFailed
		_, rbErr := e.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, obj.ETag)
		if rbErr != nil {
			return nil, rbErr
		}
		return nil, err
	}

	respCode := http.StatusCreated
	if req.Method == http.MethodPatch {
		respCode = http.StatusAccepted
	}

	return rest.NewAsyncOperationResponse(newResource, newResource.TrackedResource.Location, respCode,
		serviceCtx.ResourceID, serviceCtx.OperationID, serviceCtx.APIVersion), nil

}

// Validate extracts versioned resource from request and validate the properties.
func (e *CreateOrUpdateGateway) Validate(ctx context.Context, req *http.Request, apiVersion string) (*datamodel.Gateway, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	content, err := ctrl.ReadJSONBody(req)
	if err != nil {
		return nil, err
	}

	dm, err := converter.GatewayDataModelFromVersioned(content, apiVersion)
	if err != nil {
		return nil, err
	}

	dm.ID = serviceCtx.ResourceID.String()
	dm.TrackedResource = ctrl.BuildTrackedResource(ctx)

	return dm, err
}

// enrichMetadata updates the gateway resource before it is saved to the DB.
func enrichMetadata(serviceCtx *servicecontext.ARMRequestContext, er *datamodel.Gateway, nr *datamodel.Gateway) {

	nr.SystemData = ctrl.UpdateSystemData(er.SystemData, *serviceCtx.SystemData())
	if er.InternalMetadata.CreatedAPIVersion != "" {
		nr.InternalMetadata.CreatedAPIVersion = er.InternalMetadata.CreatedAPIVersion
	}
	nr.InternalMetadata.TenantID = serviceCtx.HomeTenantID
	nr.Properties.ProvisioningState = v1.ProvisioningStateAccepted
}
