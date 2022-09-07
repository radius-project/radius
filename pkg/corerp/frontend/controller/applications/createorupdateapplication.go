// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package applications

import (
	"context"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
)

var _ ctrl.Controller = (*CreateOrUpdateApplication)(nil)

// CreateOrUpdateApplication is the controller implementation to create or update application resource.
type CreateOrUpdateApplication struct {
	ctrl.Operation[*datamodel.Application, datamodel.Application]
}

// NewCreateOrUpdateApplication creates a new instance of CreateOrUpdateApplication.
func NewCreateOrUpdateApplication(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateApplication{
		ctrl.NewOperation(opts, converter.ApplicationDataModelFromVersioned, converter.ApplicationDataModelToVersioned),
	}, nil
}

// Run executes CreateOrUpdateApplication operation.
func (a *CreateOrUpdateApplication) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	newResource, err := a.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	old, etag, isNewResource, err := a.GetResourceFromStore(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}
	if err := a.ValidateResource(ctx, req, newResource, old, etag, isNewResource); err != nil {
		return nil, err
	}

	if isNewResource {
		newResource.UpdateMetadata(serviceCtx, nil)
	} else {
		newResource.UpdateMetadata(serviceCtx, &old.SystemData)
		if err := a.ValidateLinkedResource(serviceCtx.ResourceID, isNewResource, &newResource.Properties.BasicResourceProperties, &old.Properties.BasicResourceProperties); err != nil {
			return nil, err
		}
	}
	newResource.Properties.ProvisioningState = v1.ProvisioningStateSucceeded

	nr, err := a.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return a.ConstructSyncResponse(ctx, req.Method, nr.ETag, newResource)
}
