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

func (c *CreateOrUpdateResource) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	// deploy will be called here

	return ctrl.Result{}, nil
}
