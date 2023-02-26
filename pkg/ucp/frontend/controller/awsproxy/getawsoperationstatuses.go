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
	"github.com/project-radius/radius/pkg/ucp/aws/servicecontext"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
)

var _ ctrl.Controller = (*GetAWSOperationStatuses)(nil)

// GetAWSOperationStatuses is the controller implementation to get AWS resource operation status.
type GetAWSOperationStatuses struct {
	ctrl.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	*AWSOptions
}

// NewGetAWSOperationStatuses creates a new GetAWSOperationStatuses.
func NewGetAWSOperationStatuses(awsOpts *AWSOptions) (ctrl.Controller, error) {
	return &GetAWSOperationStatuses{
		ctrl.NewOperation(awsOpts.Options,
			ctrl.ResourceOptions[datamodel.AWSResource]{},
		),
		awsOpts,
	}, nil
}

func (p *GetAWSOperationStatuses) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	// serviceCtx := servicecontext.AWSRequestContextFromContext(ctx)
	resourceType, id, err := ParseAWSRequest(ctx, p.AWSOptions, req)
	serviceCtx := servicecontext.AWSRequestContext{}
	serviceCtx.ResourceID = id
	serviceCtx.ResourceType = resourceType

	response, err := p.AWSOptions.AWSCloudControlClient.GetResourceRequestStatus(ctx, &cloudcontrol.GetResourceRequestStatusInput{
		RequestToken: aws.String(serviceCtx.ResourceID.Name()),
	})
	if awsclient.IsAWSResourceNotFound(err) {
		return armrpc_rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
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
