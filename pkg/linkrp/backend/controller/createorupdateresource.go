// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"
	"errors"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*CreateOrUpdateResource)(nil)

// CreateOrUpdateResource is the async operation controller to create or update Applications.Link resources.
type CreateOrUpdateResource struct {
	ctrl.BaseController
}

// NewCreateOrUpdateResource creates the CreateOrUpdateResource controller instance.
func NewCreateOrUpdateResource(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateResource{ctrl.NewBaseAsyncController(opts)}, nil
}

func (c *CreateOrUpdateResource) Run(ctx context.Context, req *ctrl.Request) (ctrl.Result, error) {
	obj, err := c.StorageClient().Get(ctx, req.ResourceID)
	if err != nil && !errors.Is(&store.ErrNotFound{}, err) {
		return ctrl.Result{}, err
	}

	isNewResource := false
	if errors.Is(&store.ErrNotFound{}, err) {
		isNewResource = true
	}

	opType, _ := v1.ParseOperationType(req.OperationType)
	if opType.Method == http.MethodPatch && errors.Is(&store.ErrNotFound{}, err) {
		return ctrl.Result{}, err
	}

	// This code is general and we might be processing an async job for a resource or a scope, so using the general Parse function.
	id, err := resources.Parse(req.ResourceID)
	if err != nil {
		return ctrl.Result{}, err
	}

	dataModel, err := getDataModel(id)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err = obj.As(dataModel); err != nil {
		return ctrl.Result{}, err
	}

	rendererOutput, err := c.LinkDeploymentProcessor().Render(ctx, id, dataModel)
	if err != nil {
		return ctrl.Result{}, err
	}

	deploymentOutput, err := c.LinkDeploymentProcessor().Deploy(ctx, id, rendererOutput)
	if err != nil {
		return ctrl.Result{}, err
	}

	deploymentDataModel, ok := dataModel.(rpv1.DeploymentDataModel)
	if !ok {
		return ctrl.NewFailedResult(v1.ErrorDetails{Message: "deployment data model conversion error"}), err
	}

	oldOutputResources := deploymentDataModel.OutputResources()
	err = deploymentDataModel.ApplyDeploymentOutput(deploymentOutput)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !isNewResource {
		diff := rpv1.GetGCOutputResources(deploymentDataModel.OutputResources(), oldOutputResources)
		err = c.LinkDeploymentProcessor().Delete(ctx, id, diff)
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	nr := &store.Object{
		Metadata: store.Metadata{
			ID: req.ResourceID,
		},
		Data: deploymentDataModel,
	}
	err = c.StorageClient().Save(ctx, nr, store.WithETag(obj.ETag))
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, err
}
