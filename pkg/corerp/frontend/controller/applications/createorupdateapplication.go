// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package applications

import (
	"context"
	"errors"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*CreateOrUpdateApplication)(nil)

// CreateOrUpdateApplication is the controller implementation to create or update application resource.
type CreateOrUpdateApplication struct {
	ctrl.BaseController
}

// NewCreateOrUpdateApplication creates a new instance of CreateOrUpdateApplication.
func NewCreateOrUpdateApplication(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateApplication{ctrl.NewBaseController(opts)}, nil
}

// Run executes CreateOrUpdateApplication operation.
func (a *CreateOrUpdateApplication) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	newResource, err := a.Validate(ctx, req, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	// Read existing application resource info from the data store
	existingResource := &datamodel.Application{}

	etag, err := a.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
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

	nr, err := a.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	versioned, err := converter.ApplicationDataModelToVersioned(newResource, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{"ETag": nr.ETag}

	return rest.NewOKResponseWithHeaders(versioned, headers), nil
}

// Validate extracts versioned resource from request and validates the properties.
func (a *CreateOrUpdateApplication) Validate(ctx context.Context, req *http.Request, apiVersion string) (*datamodel.Application, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	content, err := ctrl.ReadJSONBody(req)
	if err != nil {
		return nil, err
	}

	dm, err := converter.ApplicationDataModelFromVersioned(content, apiVersion)
	if err != nil {
		return nil, err
	}

	dm.ID = serviceCtx.ResourceID.String()
	dm.TrackedResource = ctrl.BuildTrackedResource(ctx)
	dm.Properties.ProvisioningState = v1.ProvisioningStateSucceeded
	return dm, nil
}

// UpdateExistingResourceData updates the application resource before it is saved to the DB.
func UpdateExistingResourceData(ctx context.Context, er *datamodel.Application, nr *datamodel.Application) {
	sc := servicecontext.ARMRequestContextFromContext(ctx)
	nr.SystemData = ctrl.UpdateSystemData(er.SystemData, *sc.SystemData())
	if er.InternalMetadata.CreatedAPIVersion != "" {
		nr.InternalMetadata.CreatedAPIVersion = er.InternalMetadata.CreatedAPIVersion
	}
	nr.InternalMetadata.TenantID = sc.HomeTenantID
}
