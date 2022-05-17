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
	"github.com/project-radius/radius/pkg/store"
)

var _ ctrl.ControllerInterface = (*GetOperationStatus)(nil)

// GetOperationResult is the controller implementation to get the result of an async operation.
type GetOperationResult struct {
	ctrl.BaseController
}

// NewGetOperationResult creates a new GetOperationResult.
func NewGetOperationResult(storageClient store.StorageClient, jobEngine deployment.DeploymentProcessor) (ctrl.ControllerInterface, error) {
	return &GetOperationResult{
		BaseController: ctrl.BaseController{
			DBClient:  storageClient,
			JobEngine: jobEngine,
		},
	}, nil
}

// Run returns the response with necessary headers about the async operation.
// Spec: https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/async-api-reference.md#azure-asyncoperation-resource-format
func (e *GetOperationResult) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	os := &asyncoperation.AsyncOperationStatus{}
	_, err := e.GetResource(ctx, serviceCtx.ResourceID.ID, os)
	if err != nil && errors.Is(&store.ErrNotFound{}, err) {
		return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}

	// TODO: How are we going to decide on 204 or 200
	// TODO: If the resource is deleted we don't have a ProvisioningState to represent that
	if !os.InTerminalState() {
		resp := rest.NewAcceptedAsyncResponse(nil, req.URL.String(), req.URL.Scheme)
		return resp, nil
	}

	return rest.NewOKResponse(os.AsyncOperationStatus), nil
}
