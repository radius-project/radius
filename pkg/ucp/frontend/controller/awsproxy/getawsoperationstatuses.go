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
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
)

var _ ctrl.Controller = (*GetAWSOperationStatuses)(nil)

// GetAWSOperationStatuses is the controller implementation to get AWS resource operation status.
type GetAWSOperationStatuses struct {
	ctrl.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	AWSOptions
}

// NewGetAWSOperationStatuses creates a new GetAWSOperationStatuses.
func NewGetAWSOperationStatuses(opts ctrl.Options, awsOpts AWSOptions) (ctrl.Controller, error) {
	return &GetAWSOperationStatuses{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.AWSResource]{},
		),
		awsOpts,
	}, nil
}

func (p *GetAWSOperationStatuses) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	_, id, err := ParseAWSRequest(ctx, *p.Options(), p.AWSOptions, req)
	if err != nil {
		return nil, err
	}

	response, err := p.AWSOptions.AWSCloudControlClient.GetResourceRequestStatus(ctx, &cloudcontrol.GetResourceRequestStatusInput{
		RequestToken: aws.String(id.Name()),
	})
	if awsclient.IsAWSResourceNotFound(err) {
		return armrpc_rest.NewNotFoundResponse(id), nil
	} else if err != nil {
		return awsclient.HandleAWSError(err)
	}

	opStatus := getAsyncOperationStatus(response)
	return armrpc_rest.NewOKResponse(opStatus), nil
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
