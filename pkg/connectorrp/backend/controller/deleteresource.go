// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"

	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/project-radius/radius/pkg/deployment"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*DeleteResource)(nil)

// DeleteResource is the base backend controller to delete the given resource.
type DeleteResource struct {
	ctrl.BaseController
	deployment.DeploymentProcessor
}

// NewDeleteResource creates the DeleteResource controller instance.
func NewDeleteResource(store store.StorageClient, dp deployment.DeploymentProcessor) (ctrl.Controller, error) {
	// TODO: Why do we need to get the base from NewBaseAsyncController?
	return &DeleteResource{ctrl.NewBaseAsyncController(store), dp}, nil
}

func (d *DeleteResource) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	// deploy will be called here

	return ctrl.Result{}, nil
}
