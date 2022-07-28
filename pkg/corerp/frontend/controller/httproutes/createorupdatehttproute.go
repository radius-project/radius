// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package httproutes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var (
	_ ctrl.Controller = (*CreateOrUpdateHTTPRoute)(nil)

	// AsyncPutHTTPRouteOperationTimeout is the default timeout duration of async put httproute operation.
	AsyncPutHTTPRouteOperationTimeout = time.Duration(120) * time.Second
)

// CreateOrUpdateHTTPRoute is the controller implementation to create or update HTTPRoute resource.
type CreateOrUpdateHTTPRoute struct {
	ctrl.BaseController
}

// NewCreateOrUpdateTTPRoute creates a new CreateOrUpdateHTTPRoute.
func NewCreateOrUpdateHTTPRoute(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateHTTPRoute{ctrl.NewBaseController(opts)}, nil
}

// Run executes CreateOrUpdateHTTPRoute operation.
func (e *CreateOrUpdateHTTPRoute) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	newResource, err := e.Validate(ctx, req, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	old := &datamodel.HTTPRoute{}
	isNewResource := false
	etag, err := e.GetResource(ctx, serviceCtx.ResourceID.String(), old)
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			isNewResource = true
		} else {
			return nil, err
		}
	}
	if req.Method == http.MethodPatch && isNewResource {
		return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}
	if !isNewResource && !old.Properties.ProvisioningState.IsTerminal() {
		return rest.NewConflictResponse(fmt.Sprintf(ctrl.InProgressStateMessageFormat, old.Properties.ProvisioningState)), nil
	}

	err = ctrl.ValidateETag(*serviceCtx, etag)
	if err != nil {
		return rest.NewPreconditionFailedResponse(serviceCtx.ResourceID.String(), err.Error()), nil
	}

	newResource.SystemData = ctrl.UpdateSystemData(old.SystemData, *serviceCtx.SystemData())
	if !isNewResource {
		newResource.CreatedAPIVersion = old.CreatedAPIVersion
		prop := newResource.Properties.BasicResourceProperties
		if !old.Properties.BasicResourceProperties.EqualLinkedResource(prop) {
			return rest.NewLinkedResourceUpdateErrorResponse(serviceCtx.ResourceID.String(), &old.Properties.BasicResourceProperties), nil
		}
	}

	nr, err := e.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	err = e.StatusManager().QueueAsyncOperation(ctx, serviceCtx, AsyncPutHTTPRouteOperationTimeout)
	if err != nil {
		newResource.Properties.ProvisioningState = v1.ProvisioningStateFailed
		_, rbErr := e.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, nr.ETag)
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
func (e *CreateOrUpdateHTTPRoute) Validate(ctx context.Context, req *http.Request, apiVersion string) (*datamodel.HTTPRoute, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	content, err := ctrl.ReadJSONBody(req)
	if err != nil {
		return nil, err
	}

	dm, err := converter.HTTPRouteDataModelFromVersioned(content, apiVersion)
	if err != nil {
		return nil, err
	}

	dm.ID = serviceCtx.ResourceID.String()
	dm.TrackedResource = ctrl.BuildTrackedResource(ctx)
	dm.Properties.ProvisioningState = v1.ProvisioningStateAccepted
	dm.TenantID = serviceCtx.HomeTenantID
	dm.CreatedAPIVersion = dm.UpdatedAPIVersion

	return dm, err
}
