// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"
	"errors"

	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/deployment"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*CreateOrUpdateResource)(nil)

// CreateOrUpdateResource is the base backend controller to create or update the given resource.
type CreateOrUpdateResource struct {
	ctrl.BaseController
	deployment.DeploymentProcessor
}

// NewCreateOrUpdateResource creates the CreateOrUpdateResource controller instance.
func NewCreateOrUpdateResource(store store.StorageClient, dp deployment.DeploymentProcessor) (ctrl.Controller, error) {
	// TODO: Why do we need to get the base from NewBaseAsyncController?
	return &CreateOrUpdateResource{ctrl.NewBaseAsyncController(store), dp}, nil
}

func (r *CreateOrUpdateResource) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	// TODO: Integration with modified new backend controller
	// Note: mentioning here in this current placeholder controller to show the flow, it will not be checked in,
	// The job of backend controller is to do two major operations 1. Render and 2. Deploy

	// dataprovider.NewStorageProvider()

	// Get the resource
	existingResource := &datamodel.ContainerResource{}
	etag, err := r.GetResource(ctx, request.ResourceID, existingResource)
	if err != nil && !errors.Is(&store.ErrNotFound{}, err) {
		return ctrl.Result{}, err
	}
	id, err := resources.Parse(request.ResourceID)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Render the resource
	rendererOutput, err := r.DeploymentProcessor.Render(ctx, id, existingResource)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Deploy the resource
	deploymentOutput, err := r.DeploymentProcessor.Deploy(ctx, id, rendererOutput)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Update the resource with deployed outputResources
	existingResource.Properties.BasicResourceProperties.Status.OutputResources = deploymentOutput.Resources
	existingResource.InternalMetadata.ComputedValues = deploymentOutput.ComputedValues
	existingResource.InternalMetadata.SecretValues = deploymentOutput.SecretValues

	// Save the resource
	_, err = r.SaveResource(ctx, request.ResourceID, existingResource, etag)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
