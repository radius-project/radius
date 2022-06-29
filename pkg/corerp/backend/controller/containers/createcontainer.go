// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containers

import (
	"context"
	"errors"

	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/project-radius/radius/pkg/corerp/backend/deployment"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/model"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*UpdateContainer)(nil)

// UpdateContainer is the async operation controller to create or update Applications.Core/Containers resource.
type UpdateContainer struct {
	ctrl.BaseController
}

// NewUpdateContainer creates the UpdateContainer controller instance.
func NewUpdateContainer(store store.StorageClient) (ctrl.Controller, error) {
	return &UpdateContainer{ctrl.NewBaseAsyncController(store)}, nil
}

func (c *UpdateContainer) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	// TODO: Integration with modified new backend controller
	// Note: mentioning here in this current placeholder controller to show the flow, it will not be checked in,
	// The job of backend controller is to do two major operations 1. Render and 2. Deploy
	dp := deployment.NewDeploymentProcessor(model.ApplicationModel{}, dataprovider.NewStorageProvider(dataprovider.StorageProviderOptions{}), nil, nil)

	// Get the resource
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
	rendererOutput, err := dp.Render(ctx, id, existingResource)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Deploy the resource
	deploymentOutput, err := dp.Deploy(ctx, id, rendererOutput)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Update the resource with deployed outputResources
	deployedOuputResources := deploymentOutput.DeployedOutputResources
	var outputResources []map[string]interface{}
	for _, deployedOutputResource := range deployedOuputResources {
		outputResource := map[string]interface{}{
			deployedOutputResource.LocalID: deployedOutputResource,
		}
		outputResources = append(outputResources, outputResource)
	}
	existingResource.Properties.BasicResourceProperties.Status.OutputResources = outputResources
	existingResource.ComputedValues = deploymentOutput.ComputedValues
	existingResource.SecretValues = deploymentOutput.SecretValues

	// Save the resource
	_, err = c.SaveResource(ctx, request.ResourceID, existingResource, etag)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
