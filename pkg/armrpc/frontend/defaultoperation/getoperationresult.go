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
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
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
func (e *GetOperationResult) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

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
			"Retry-After": fmt.Sprintf("%v", os.RetryAfter.Truncate(time.Second).Seconds()),
		}
		return rest.NewAsyncOperationResultResponse(headers), nil
	}

	return rest.NewNoContentResponse(), nil
}

// getOperationStatusResourceID function gets the operationResults resourceID
// and converts it to an operationStatuses resourceID.
func getOperationStatusResourceID(resourceID string) (resources.ID, error) {
	id, err := resources.ParseResource(resourceID)
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
