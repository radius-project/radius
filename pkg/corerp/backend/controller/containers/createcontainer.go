// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containers

import (
	"context"

	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
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
	// TODO: Implement Create or Update Container async operation.

	// Should we return Succeeded here based on the output of the provisioning?
	return ctrl.Result{}, nil
}
