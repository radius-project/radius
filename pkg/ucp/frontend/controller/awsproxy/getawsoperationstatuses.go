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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	cctypes "github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ucpaws "github.com/radius-project/radius/pkg/ucp/aws"

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
	awsClients ucpaws.Clients
}

// NewGetAWSOperationStatuses creates a new GetAWSOperationStatuses controller which is used to get the statuses of AWS operations.
func NewGetAWSOperationStatuses(opts armrpc_controller.Options, awsClients ucpaws.Clients) (armrpc_controller.Controller, error) {
	return &GetAWSOperationStatuses{
		Operation:  armrpc_controller.NewOperation(opts, armrpc_controller.ResourceOptions[datamodel.AWSResource]{}),
		awsClients: awsClients,
	}, nil
}

// Run returns the status of an AWS resource operation. UCP owns every create, update, and delete by alias, so their
// status is in UCP's data store.
//
// Transitional: an operation that UCP's data store does not have was started by POST /:put or /:delete, which do not
// track operations. Its ID is the Cloud Control request token, so Cloud Control is asked, as before resource records.
// This fallback is removed with those routes.
func (p *GetAWSOperationStatuses) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	serviceCtx := servicecontext.AWSRequestContextFromContext(ctx)
	region, errResponse := readRegionFromRequest(req.URL.Path, p.Options().PathBase)
	if errResponse != nil {
		return errResponse, nil
	}

	status, err := getOperationStatus(ctx, p.Options().StatusManager, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	} else if status != nil {
		return armrpc_rest.NewOKResponse(status.AsyncOperationStatus), nil
	}

	response, err := p.awsClients.CloudControl.GetResourceRequestStatus(ctx, &cloudcontrol.GetResourceRequestStatusInput{
		RequestToken: aws.String(serviceCtx.ResourceID.Name()),
	}, CloudControlRegionOption(region))
	if ucpaws.IsAWSResourceNotFoundError(err) {
		return armrpc_rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	} else if err != nil {
		return ucpaws.HandleAWSError(ctx, err)
	}

	// A delete of a resource that no longer exists is done.
	if response.ProgressEvent != nil &&
		response.ProgressEvent.Operation == cctypes.OperationDelete &&
		response.ProgressEvent.ErrorCode == cctypes.HandlerErrorCodeNotFound {
		return armrpc_rest.NewNoContentResponse(), nil
	}
	return armrpc_rest.NewOKResponse(getAsyncOperationStatus(response)), nil
}

func getAsyncOperationStatus(response *cloudcontrol.GetResourceRequestStatusOutput) v1.AsyncOperationStatus {
	os := statusmanager.Status{}
	switch response.ProgressEvent.OperationStatus {
	case cctypes.OperationStatusSuccess:
		os.AsyncOperationStatus.Status = v1.ProvisioningStateSucceeded
	case cctypes.OperationStatusCancelComplete:
		os.AsyncOperationStatus.Status = v1.ProvisioningStateCanceled
	case cctypes.OperationStatusFailed:
		os.AsyncOperationStatus.Status = v1.ProvisioningStateFailed
	default:
		os.AsyncOperationStatus.Status = v1.ProvisioningStateProvisioning
	}
	os.AsyncOperationStatus.StartTime = aws.ToTime(response.ProgressEvent.EventTime)
	if response.ProgressEvent.OperationStatus == cctypes.OperationStatusFailed {
		os.Error = &v1.ErrorDetails{
			Code:    string(response.ProgressEvent.ErrorCode),
			Message: aws.ToString(response.ProgressEvent.StatusMessage),
		}
	}
	return os.AsyncOperationStatus
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
