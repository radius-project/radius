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

type CreateContainerController struct {
	asyncoperation.BaseController
}

func NewCreateContainerController(store store.StorageClient) (asyncoperation.Controller, error) {
	return &CreateContainerController{
		BaseController: asyncoperation.NewBaseAsyncController(store),
	}, nil
}

func (ctrl *CreateContainerController) Run(ctx context.Context, request *asyncoperation.Request) (asyncoperation.Result, error) {
	return asyncoperation.Result{}, nil
}
