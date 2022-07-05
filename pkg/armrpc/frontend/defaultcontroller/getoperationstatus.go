// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package defaultoperation

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/connectorrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*GetOperationStatus)(nil)

// GetOperationStatus is the controller implementation to get an async operation status.
type GetOperationStatus struct {
	ctrl.BaseController
}

// NewGetOperationStatus creates a new GetOperationStatus.
func NewGetOperationStatus(ds store.StorageClient, sm manager.StatusManager, dp deployment.DeploymentProcessor) (ctrl.Controller, error) {
	return &GetOperationStatus{ctrl.NewBaseController(ds, sm, dp)}, nil
}

// Run returns the async operation status.
// Spec: https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/async-api-reference.md#azure-asyncoperation-resource-format
func (e *GetOperationStatus) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	fmt.Println("In get operation status")
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	// TODO: Add additional validation

	os := &manager.Status{}

	fmt.Println(serviceCtx.ResourceID.String())

	_, err := e.GetResource(ctx, serviceCtx.ResourceID.String(), os)
	if err != nil && errors.Is(&store.ErrNotFound{}, err) {
		fmt.Println("Failed to get resource, 404")
		return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}

	return rest.NewOKResponse(os.AsyncOperationStatus), nil
}
