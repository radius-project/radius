// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package defaultoperation

import (
	"context"
	"errors"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*GetOperationResult)(nil)

// GetOperationResult is the controller implementation to get the result of an async operation.
type GetOperationResult struct {
	ctrl.BaseController
}

// NewGetOperationResult creates a new GetOperationResult.
func NewGetOperationResult(opts ctrl.Options) (ctrl.Controller, error) {
	return &GetOperationResult{ctrl.NewBaseController(opts)}, nil
}

// Run returns the response with necessary headers about the async operation.
// Spec: https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/async-api-reference.md#azure-asyncoperation-resource-format
func (e *GetOperationResult) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	id, err := getOperationStatusResourceID(serviceCtx.ResourceID.String())
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	os := &manager.Status{}
	_, err = e.GetResource(ctx, id.String(), os)
	if err != nil && errors.Is(&store.ErrNotFound{}, err) {
		return rest.NewNotFoundResponse(id), nil
	}

	if !os.Status.IsTerminal() {
		headers := map[string]string{
			"Location":    req.URL.String(),
			"Retry-After": v1.DefaultRetryAfter,
		}
		return rest.NewAsyncOperationResultResponse(headers), nil
	}

	return rest.NewNoContentResponse(), nil
}

// getOperationStatusResourceID function gets the operationResults resourceID
// and converts it to an operationStatuses resourceID.
func getOperationStatusResourceID(resourceID string) (resources.ID, error) {
	id, err := resources.Parse(resourceID)
	if err != nil {
		return id, err
	}

	typeSegments := id.TypeSegments()
	lastSegment := typeSegments[len(typeSegments)-1]
	osTypeSegment := resources.TypeSegment{
		Type: "operationstatuses",
		Name: lastSegment.Name,
	}

	id = id.Truncate().
		Append(osTypeSegment)

	return id, nil
}
