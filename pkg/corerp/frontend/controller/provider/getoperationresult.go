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
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
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

	id, err := getOperationStatusResourceID(serviceCtx.ResourceID.String())
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	os := &asyncoperation.Status{}
	_, err = e.GetResource(ctx, id.String(), os)
	if err != nil && errors.Is(&store.ErrNotFound{}, err) {
		return rest.NewNotFoundResponse(id), nil
	}

	if !os.InTerminalState() {
		headers := map[string]string{
			"Location":    req.URL.String(),
			"Retry-After": asyncoperation.DefaultRetryAfter,
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
