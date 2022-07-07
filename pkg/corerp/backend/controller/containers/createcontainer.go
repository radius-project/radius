// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containers

import (
	"context"
	"errors"

	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*UpdateContainer)(nil)

// UpdateContainer is the async operation controller to create or update Applications.Core/Containers resource.
type UpdateContainer struct {
	ctrl.BaseController
}

// NewUpdateContainer creates the UpdateContainer controller instance.
func NewUpdateContainer(opts ctrl.Options) (ctrl.Controller, error) {
	return &UpdateContainer{ctrl.NewBaseAsyncController(opts)}, nil
}

func (c *UpdateContainer) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	existingResource := &datamodel.ContainerResource{}
	etag, err := c.GetResource(ctx, request.ResourceID, existingResource)
	if err != nil && !errors.Is(&store.ErrNotFound{}, err) {
		return ctrl.Result{}, err
	}
	id, err := resources.Parse(request.ResourceID)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Render the resource
	rendererOutput, err := c.DeploymentProcessor().Render(ctx, id, *existingResource)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Deploy the resource
	deploymentOutput, err := c.DeploymentProcessor().Deploy(ctx, id, rendererOutput)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Update the resource with deployed outputResources
	existingResource.Properties.BasicResourceProperties.Status.OutputResources = deploymentOutput.DeployedOutputResources
	existingResource.InternalMetadata.ComputedValues = deploymentOutput.ComputedValues
	existingResource.InternalMetadata.SecretValues = deploymentOutput.SecretValues

	// Save the resource
	_, err = c.SaveResource(ctx, request.ResourceID, existingResource, etag)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
