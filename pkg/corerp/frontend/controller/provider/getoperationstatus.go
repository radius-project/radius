// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"context"
	"errors"
	"net/http"

	"github.com/project-radius/radius/pkg/corerp/asyncoperation"
	ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.ControllerInterface = (*GetOperationStatus)(nil)

// GetOperationStatus is the controller implementation to get an async operation status.
type GetOperationStatus struct {
	ctrl.BaseController
}

// NewGetOperationStatus creates a new GetOperationStatus.
func NewGetOperationStatus(storageClient store.StorageClient, jobEngine deployment.DeploymentProcessor) (ctrl.ControllerInterface, error) {
	return &GetOperationStatus{
		BaseController: ctrl.BaseController{
			DBClient:  storageClient,
			JobEngine: jobEngine,
		},
	}, nil
}

// Run returns the async operation status.
// Spec: https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/async-api-reference.md#azure-asyncoperation-resource-format
func (e *GetOperationStatus) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	// TODO: Add additional validation

	os := &asyncoperation.AsyncOperationStatus{}
	_, err := e.GetResource(ctx, serviceCtx.ResourceID.String(), os)
	if err != nil && errors.Is(&store.ErrNotFound{}, err) {
		return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}

	return rest.NewOKResponse(os.AsyncOperationStatus), nil
}
