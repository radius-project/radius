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
	http "net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	armrpcv1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	manager "github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	awsclient "github.com/radius-project/radius/pkg/ucp/aws"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/servicecontext"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

var _ armrpc_controller.Controller = (*GetAWSOperationStatuses)(nil)

// GetAWSOperationStatuses is the controller implementation to get AWS resource operation status.
type GetAWSOperationStatuses struct {
	armrpc_controller.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	awsClients ucp_aws.Clients
}

// NewGetAWSOperationStatuses creates a new GetAWSOperationStatuses controller which is used to get the statuses of AWS operations.
func NewGetAWSOperationStatuses(opts armrpc_controller.Options, awsClients ucp_aws.Clients) (armrpc_controller.Controller, error) {
	return &GetAWSOperationStatuses{
		Operation:  armrpc_controller.NewOperation(opts, armrpc_controller.ResourceOptions[datamodel.AWSResource]{}),
		awsClients: awsClients,
	}, nil
}

// Run() reads the region from the request, uses the region to get the resource request status
// from AWS CloudControl, and returns the async operation status. If the resource is not found, it returns a
// NotFoundResponse, and if there is an error, it returns an error response.
func (p *GetAWSOperationStatuses) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	serviceCtx := servicecontext.AWSRequestContextFromContext(ctx)
	region, errResponse := readRegionFromRequest(req.URL.Path, p.Options().PathBase)
	if errResponse != nil {
		return errResponse, nil
	}

	cloudControlOpts := []func(*cloudcontrol.Options){CloudControlRegionOption(region)}
	response, err := p.awsClients.CloudControl.GetResourceRequestStatus(ctx, &cloudcontrol.GetResourceRequestStatusInput{
		RequestToken: aws.String(serviceCtx.ResourceID.Name()),
	}, cloudControlOpts...)
	if awsclient.IsAWSResourceNotFoundError(err) {
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
