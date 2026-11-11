/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package awsproxy

import (
	"context"
	"errors"
	http "net/http"

	"uuid"

	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/ucp/aws/servicecontext"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

var _ armrpc_controller.Controller = (*GetAWSOperationStatuses)(nil)

// GetAWSOperationStatuses is the controller implementation to get AWS resource operation status.
type GetAWSOperationStatuses struct {
	armrpc_controller.Operation[*datamodel.AWSResource, datamodel.AWSResource]
}

// NewGetAWSOperationStatuses creates a new GetAWSOperationStatuses controller which is used to get the statuses of AWS operations.
func NewGetAWSOperationStatuses(opts armrpc_controller.Options) (armrpc_controller.Controller, error) {
	return &GetAWSOperationStatuses{
		Operation: armrpc_controller.NewOperation(opts, armrpc_controller.ResourceOptions[datamodel.AWSResource]{}),
	}, nil
}

// Run returns the status of an AWS resource operation. UCP owns every create, update, and delete of an AWS resource,
// so the status is in UCP's data store. An operation that is not there returns 404.
func (p *GetAWSOperationStatuses) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	serviceCtx := servicecontext.AWSRequestContextFromContext(ctx)
	if _, errResponse := readRegionFromRequest(req.URL.Path, p.Options().PathBase); errResponse != nil {
		return errResponse, nil
	}

	status, err := getOperationStatus(ctx, p.Options().StatusManager, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	} else if status == nil {
		return armrpc_rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}
	return armrpc_rest.NewOKResponse(status.AsyncOperationStatus), nil
}

// getOperationStatus reads the status of the operation whose operationStatuses or operationResults ID is
// operationStatusID. It returns nil when there is no such operation.
func getOperationStatus(ctx context.Context, sm statusmanager.StatusManager, operationStatusID resources.ID) (*statusmanager.Status, error) {
	operationID, err := uuid.Parse(operationStatusID.Name())
	if err != nil {
		return nil, nil
	}
	if sm == nil {
		return nil, errors.New("async operations are not available: the status manager is not configured")
	}

	status, err := sm.Get(ctx, operationStatusID, operationID)
	if errors.Is(err, &database.ErrNotFound{}) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return status, nil
}
