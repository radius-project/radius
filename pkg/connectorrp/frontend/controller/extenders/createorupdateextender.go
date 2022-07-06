// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package extenders

import (
	"context"
	"errors"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*CreateOrUpdateExtender)(nil)

// CreateOrUpdateExtender is the controller implementation to create or update Extender connector resource.
type CreateOrUpdateExtender struct {
	ctrl.BaseController
}

// NewCreateOrUpdateExtender creates a new instance of CreateOrUpdateExtender.
func NewCreateOrUpdateExtender(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateExtender{ctrl.NewBaseController(opts)}, nil
}

// Run executes CreateOrUpdateExtender operation.
func (extender *CreateOrUpdateExtender) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	newResource, err := extender.Validate(ctx, req, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	rendererOutput, err := extender.DeploymentProcessor().Render(ctx, serviceCtx.ResourceID, newResource)
	if err != nil {
		return nil, err
	}
	deploymentOutput, err := extender.DeploymentProcessor().Deploy(ctx, serviceCtx.ResourceID, rendererOutput)
	if err != nil {
		return nil, err
	}

	newResource.Properties.BasicResourceProperties.Status.OutputResources = deploymentOutput.Resources
	newResource.InternalMetadata.ComputedValues = deploymentOutput.ComputedValues
	newResource.InternalMetadata.SecretValues = deploymentOutput.SecretValues

	// Read existing resource info from the data store
	existingResource := &datamodel.Extender{}
	etag, err := extender.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if req.Method == http.MethodPatch && err != nil && !errors.Is(&store.ErrNotFound{}, err) {
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
	savedResource, err := extender.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	versioned, err := converter.ExtenderDataModelToVersioned(newResource, serviceCtx.APIVersion, true)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{"ETag": savedResource.ETag}

	return rest.NewOKResponseWithHeaders(versioned, headers), nil
}

// Validate extracts versioned resource from request and validates the properties.
func (extender *CreateOrUpdateExtender) Validate(ctx context.Context, req *http.Request, apiVersion string) (*datamodel.Extender, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	content, err := ctrl.ReadJSONBody(req)
	if err != nil {
		return nil, err
	}

	dm, err := converter.ExtenderDataModelFromVersioned(content, apiVersion)
	if err != nil {
		return nil, err
	}

	dm.ID = serviceCtx.ResourceID.String()
	dm.TrackedResource = ctrl.BuildTrackedResource(ctx)
	dm.Properties.ProvisioningState = v1.ProvisioningStateSucceeded

	return dm, nil
}
