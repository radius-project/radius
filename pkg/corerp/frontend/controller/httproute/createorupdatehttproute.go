// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package httproute

import (
	"context"
	"errors"
	"net/http"

	"github.com/project-radius/radius/pkg/basedatamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"

	ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"
)

var _ ctrl.ControllerInterface = (*CreateOrUpdateHTTPRoute)(nil)

// CreateOrUpdateHTTPRoute is the controller implementation to create or update HTTPRoute resource.
type CreateOrUpdateHTTPRoute struct {
	ctrl.BaseController
}

// NewCreateOrUpdateHTTPRoute creates a new CreateOrUpdateHTTPRoute.
func NewCreateOrUpdateHTTPRoute(storageClient store.StorageClient, jobEngine deployment.DeploymentProcessor) (ctrl.ControllerInterface, error) {
	return &CreateOrUpdateHTTPRoute{
		BaseController: ctrl.BaseController{
			DBClient:  storageClient,
			JobEngine: jobEngine,
		},
	}, nil
}

// Run executes CreateOrUpdateHTTPRoute operation.
func (e *CreateOrUpdateHTTPRoute) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	newResource, err := e.Validate(ctx, req, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	existingResource := &datamodel.HTTPRoute{}
	etag, err := e.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if req.Method == http.MethodPatch && errors.Is(&store.ErrNotFound{}, err) {
		return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}
	if err != nil && !errors.Is(&store.ErrNotFound{}, err) {
		return nil, err
	}

	err = ctrl.ValidateETag(*serviceCtx, etag)
	if err != nil {
		return rest.NewPreconditionFailedResponse(serviceCtx.ResourceID.String(), err.Error()), nil
	}

	UpdateExistingResourceData(ctx, existingResource, newResource)

	nr, err := e.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	versioned, err := converter.HTTPRouteDataModelToVersioned(newResource, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{"ETag": nr.ETag}

	return rest.NewOKResponseWithHeaders(versioned, headers), nil
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
	// TODO: Update the state.
	dm.Properties.ProvisioningState = basedatamodel.ProvisioningStateSucceeded

	return dm, err
}

// UpdateExistingResourceData updates the HTTPRoute resource before it is saved to the DB.
func UpdateExistingResourceData(ctx context.Context, er *datamodel.HTTPRoute, nr *datamodel.HTTPRoute) {
	sc := servicecontext.ARMRequestContextFromContext(ctx)
	nr.SystemData = ctrl.UpdateSystemData(er.SystemData, *sc.SystemData())
	if er.CreatedAPIVersion != "" {
		nr.CreatedAPIVersion = er.CreatedAPIVersion
	}
	nr.TenantID = sc.HomeTenantID
}
