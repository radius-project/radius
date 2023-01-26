// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package extenders

import (
	"context"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	frontend_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	rp_frontend "github.com/project-radius/radius/pkg/rp/frontend"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

var _ ctrl.Controller = (*CreateOrUpdateExtender)(nil)

// CreateOrUpdateExtender is the controller implementation to create or update Extender link resource.
type CreateOrUpdateExtender struct {
	ctrl.Operation[*datamodel.Extender, datamodel.Extender]
	dp deployment.DeploymentProcessor
}

// NewCreateOrUpdateExtender creates a new instance of CreateOrUpdateExtender.
func NewCreateOrUpdateExtender(opts frontend_ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateExtender{
		Operation: ctrl.NewOperation(opts.Options,
			ctrl.ResourceOptions[datamodel.Extender]{
				RequestConverter:  converter.ExtenderDataModelFromVersioned,
				ResponseConverter: converter.ExtenderDataModelToVersioned,
			}),
		dp: opts.DeployProcessor,
	}, nil
}

// Run executes CreateOrUpdateExtender operation.
func (extender *CreateOrUpdateExtender) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	newResource, err := extender.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	old, etag, err := extender.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	r, err := extender.PrepareResource(ctx, req, newResource, old, etag)
	if r != nil || err != nil {
		return r, err
	}

	r, err = rp_frontend.PrepareRadiusResource(ctx, newResource, old, extender.Options())
	if r != nil || err != nil {
		return r, err
	}

	rendererOutput, err := extender.dp.Render(ctx, serviceCtx.ResourceID, newResource)
	if err != nil {
		return nil, err
	}

	deploymentOutput, err := extender.dp.Deploy(ctx, serviceCtx.ResourceID, rendererOutput)
	if err != nil {
		return nil, err
	}

	newResource.Properties.Status.OutputResources = deploymentOutput.Resources
	newResource.ComputedValues = deploymentOutput.ComputedValues
	newResource.SecretValues = deploymentOutput.SecretValues

	if old != nil {
		diff := rpv1.GetGCOutputResources(newResource.Properties.Status.OutputResources, old.Properties.Status.OutputResources)
		err = extender.dp.Delete(ctx, deployment.ResourceData{ID: serviceCtx.ResourceID, Resource: newResource, OutputResources: diff, ComputedValues: newResource.ComputedValues, SecretValues: newResource.SecretValues, RecipeData: newResource.RecipeData})
		if err != nil {
			return nil, err
		}
	}

	newResource.SetProvisioningState(v1.ProvisioningStateSucceeded)
	newEtag, err := extender.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return extender.ConstructSyncResponse(ctx, req.Method, newEtag, newResource)
}
