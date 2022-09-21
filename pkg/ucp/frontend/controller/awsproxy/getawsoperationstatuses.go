// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package awsproxy

import (
	"context"
	http "net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	armrpcv1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	awserror "github.com/project-radius/radius/pkg/ucp/aws"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/rest"
)

var _ ctrl.Controller = (*GetAWSOperationStatuses)(nil)

// GetAWSOperationStatuses is the controller implementation to get AWS resource operation status.
type GetAWSOperationStatuses struct {
	ctrl.BaseController
}

// NewGetAWSOperationStatuses creates a new GetAWSOperationStatuses.
func NewGetAWSOperationStatuses(opts ctrl.Options) (ctrl.Controller, error) {
	return &GetAWSOperationStatuses{ctrl.NewBaseController(opts)}, nil
}

func (p *GetAWSOperationStatuses) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	client, _, id, err := ParseAWSRequest(ctx, p.Options.BasePath, req)
	if err != nil {
		return nil, err
	}

	response, err := client.GetResourceRequestStatus(ctx, &cloudcontrol.GetResourceRequestStatusInput{
		RequestToken: aws.String(id.Name()),
	})
	if awserror.IsAWSResourceNotFound(err) {
		return rest.NewNotFoundResponse(id.String()), nil
	} else if err != nil {
		return awserror.HandleAWSError(err)
	}

	opStatus := getAsyncOperationStatus(response)
	return rest.NewOKResponse(opStatus), nil
}

func getAsyncOperationStatus(response *cloudcontrol.GetResourceRequestStatusOutput) armrpcv1.AsyncOperationStatus {
	os := manager.Status{}
	switch response.ProgressEvent.OperationStatus {
	case types.OperationStatusSuccess:
		os.AsyncOperationStatus.Status = armrpcv1.ProvisioningStateSucceeded
	case types.OperationStatusCancelComplete:
		os.AsyncOperationStatus.Status = armrpcv1.ProvisioningStateCanceled
	case types.OperationStatusFailed:
		os.AsyncOperationStatus.Status = armrpcv1.ProvisioningStateFailed
	default:
		os.AsyncOperationStatus.Status = armrpcv1.ProvisioningStateProvisioning
	}
	os.AsyncOperationStatus.StartTime = *response.ProgressEvent.EventTime
	if response.ProgressEvent.OperationStatus == types.OperationStatusFailed {
		os.Error = &armrpcv1.ErrorDetails{
			Code:    string(response.ProgressEvent.ErrorCode),
			Message: *response.ProgressEvent.StatusMessage,
		}
	}
	return os.AsyncOperationStatus
}
