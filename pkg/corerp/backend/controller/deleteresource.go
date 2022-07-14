// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"

	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

var _ ctrl.Controller = (*DeleteResource)(nil)

// DeleteResource is the async operation controller to delete Applications.Core/Containers resource.
type DeleteResource struct {
	ctrl.BaseController
}

// NewDeleteResource creates the DeleteResource controller instance.
func NewDeleteResource(opts ctrl.Options) (ctrl.Controller, error) {
	return &DeleteResource{ctrl.NewBaseAsyncController(opts)}, nil
}

func (c *DeleteResource) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	obj, err := c.StorageClient().Get(ctx, request.ResourceID)
	if err != nil {
		return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: err.Error()}), err
	}

	id, err := resources.Parse(request.ResourceID)
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

	deploymentDataModel, ok := dataModel.(rp.DeploymentDataModel)
	if !ok {
		return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: "deployment data model conversion error"}), nil
	}

	err = c.DeploymentProcessor().Delete(ctx, id, deploymentDataModel.OutputResources())
	if err != nil {
		return ctrl.Result{}, err
	}

	err = c.StorageClient().Delete(ctx, request.ResourceID)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, err
}
