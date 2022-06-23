// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprinvokehttproutes

import (
	"context"
	"errors"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*CreateOrUpdateDaprInvokeHttpRoute)(nil)

// CreateOrUpdateDaprInvokeHttpRoute is the controller implementation to create or update DaprInvokeHttpRoute connector resource.
type CreateOrUpdateDaprInvokeHttpRoute struct {
	ctrl.BaseController
}

// NewCreateOrUpdateDaprInvokeHttpRoute creates a new instance of CreateOrUpdateDaprInvokeHttpRoute.
func NewCreateOrUpdateDaprInvokeHttpRoute(ds store.StorageClient, sm manager.StatusManager) (ctrl.Controller, error) {
	return &CreateOrUpdateDaprInvokeHttpRoute{ctrl.NewBaseController(ds, sm)}, nil
}

// Run executes CreateOrUpdateDaprInvokeHttpRoute operation.
func (daprHttpRoute *CreateOrUpdateDaprInvokeHttpRoute) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	newResource, err := daprHttpRoute.Validate(ctx, req, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	// TODO Integrate with renderer/deployment processor to validate associated resource existence (if fromResource is defined)
	// and store resource properties and secrets reference

	// Read existing resource info from the data store
	existingResource := &datamodel.DaprInvokeHttpRoute{}
	etag, err := daprHttpRoute.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if err != nil && !errors.Is(&store.ErrNotFound{}, err) {
		return nil, err
	}

	err = ctrl.ValidateETag(*serviceCtx, etag)
	if err != nil {
		return rest.NewPreconditionFailedResponse(serviceCtx.ResourceID.String(), err.Error()), nil
	}

	// Add system metadata to requested resource
	newResource.SystemData = ctrl.UpdateSystemData(existingResource.SystemData, *serviceCtx.SystemData())
	if existingResource.CreatedAPIVersion != "" {
		newResource.CreatedAPIVersion = existingResource.CreatedAPIVersion
	}
	newResource.TenantID = serviceCtx.HomeTenantID

	// Add/update resource in the data store
	savedResource, err := daprHttpRoute.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	versioned, err := converter.DaprInvokeHttpRouteDataModelToVersioned(newResource, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{"ETag": savedResource.ETag}

	return rest.NewOKResponseWithHeaders(versioned, headers), nil
}

// Validate extracts versioned resource from request and validates the properties.
func (daprHttpRoute *CreateOrUpdateDaprInvokeHttpRoute) Validate(ctx context.Context, req *http.Request, apiVersion string) (*datamodel.DaprInvokeHttpRoute, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	content, err := ctrl.ReadJSONBody(req)
	if err != nil {
		return nil, err
	}

	dm, err := converter.DaprInvokeHttpRouteDataModelFromVersioned(content, apiVersion)
	if err != nil {
		return nil, err
	}

	dm.ID = serviceCtx.ResourceID.String()
	dm.TrackedResource = ctrl.BuildTrackedResource(ctx)
	dm.Properties.ProvisioningState = v1.ProvisioningStateSucceeded

	return dm, nil
}
