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
	armrpcv1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/aws/servicecontext"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
)

var _ armrpc_controller.Controller = (*GetAWSOperationResults)(nil)

// GetAWSOperationResults is the controller implementation to get AWS resource operation results.
type GetAWSOperationResults struct {
	armrpc_controller.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	awsOptions ctrl.AWSOptions
	basePath   string
}

// NewGetAWSOperationResults creates a new GetAWSOperationResults.
//
// # Function Explanation
// 
//	GetAWSOperationResults creates a new controller with the given options and returns it, or an error if something goes 
//	wrong. It also sets up the Operation and AWSOptions fields of the controller.
func NewGetAWSOperationResults(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &GetAWSOperationResults{
		Operation: armrpc_controller.NewOperation(opts.Options,
			armrpc_controller.ResourceOptions[datamodel.AWSResource]{},
		),
		awsOptions: opts.AWSOptions,
		basePath:   opts.BasePath,
	}, nil
}

// # Function Explanation
// 
//	The GetAWSOperationResults function reads the region from the request URL, creates a cloud control client with the 
//	region, and then attempts to get the resource request status from AWS. If the resource is not found, a NotFoundResponse 
//	is returned. If there is an error, an error response is returned. If the status is not terminal, an 
//	AsyncOperationResultResponse is returned with a Retry-After header. Otherwise, a NoContentResponse is returned.
func (p *GetAWSOperationResults) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	serviceCtx := servicecontext.AWSRequestContextFromContext(ctx)
	region, errResponse := readRegionFromRequest(req.URL.Path, p.basePath)
	if errResponse != nil {
		return errResponse, nil
	}
	cloudControlOpts := []func(*cloudcontrol.Options){CloudControlRegionOption(region)}
	response, err := p.awsOptions.AWSCloudControlClient.GetResourceRequestStatus(ctx, &cloudcontrol.GetResourceRequestStatusInput{
		RequestToken: aws.String(serviceCtx.ResourceID.Name()),
	}, cloudControlOpts...)

	if awsclient.IsAWSResourceNotFoundError(err) {
		return armrpc_rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	} else if err != nil {
		return awsclient.HandleAWSError(err)
	}

	isTerminal := isStatusTerminal(response)

	if !isTerminal {
		headers := map[string]string{
			"Location":    req.URL.String(),
			"Retry-After": armrpcv1.DefaultRetryAfter,
		}
		return armrpc_rest.NewAsyncOperationResultResponse(headers), nil
	}

	return armrpc_rest.NewNoContentResponse(), nil
}

func isStatusTerminal(response *cloudcontrol.GetResourceRequestStatusOutput) bool {
	isTerminal := false
	switch response.ProgressEvent.OperationStatus {
	case types.OperationStatusSuccess:
		isTerminal = true
	case types.OperationStatusCancelComplete:
		isTerminal = true
	case types.OperationStatusFailed:
		isTerminal = true
	}
	return isTerminal
}
