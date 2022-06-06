// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containers

import (
	"context"

	"github.com/project-radius/radius/pkg/corerp/asyncoperation"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ asyncoperation.Controller = (*UpdateContainer)(nil)

// UpdateContainer is the async operation controller to create or update Applications.Core/Containers resource.
type UpdateContainer struct {
	asyncoperation.BaseController
}

// NewUpdateContainer creates the UpdateContainer controller instance.
func NewUpdateContainer(store store.StorageClient) (asyncoperation.Controller, error) {
	return &UpdateContainer{
		BaseController: asyncoperation.NewBaseAsyncController(store),
	}, nil
}

func (ctrl *UpdateContainer) Run(ctx context.Context, request *asyncoperation.Request) (asyncoperation.Result, error) {
	// TODO: Implement Create or Update Container async operation.

	return asyncoperation.Result{}, nil
}
