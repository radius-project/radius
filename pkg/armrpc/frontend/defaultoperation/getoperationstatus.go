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

	"github.com/go-logr/logr"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*GetOperationStatus)(nil)

// GetOperationStatus is the controller implementation to get an async operation status.
type GetOperationStatus struct {
	ctrl.BaseController
}

// NewGetOperationStatus creates a new GetOperationStatus.
func NewGetOperationStatus(opts ctrl.Options) (ctrl.Controller, error) {
	return &GetOperationStatus{ctrl.NewBaseController(opts)}, nil
}

// Run returns the async operation status.
// Spec: https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/async-api-reference.md#azure-asyncoperation-resource-format
func (e *GetOperationStatus) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	logger := logr.FromContextOrDiscard(ctx)
	// TODO: Add additional validation

	os := &manager.Status{}
	_, err := e.GetResource(ctx, serviceCtx.ResourceID.String(), os)
	if err != nil && errors.Is(&store.ErrNotFound{}, err) {
		logger.Info(fmt.Sprintf("The response is %s for resource %s", err.Error(), serviceCtx.ResourceID))
		return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}

	return rest.NewOKResponse(os.AsyncOperationStatus), nil
}
