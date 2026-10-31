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
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	ucpaws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/servicecontext"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

var _ armrpc_controller.Controller = (*GetAWSOperationResults)(nil)

// GetAWSOperationResults is the controller implementation to get AWS resource operation results.
type GetAWSOperationResults struct {
	armrpc_controller.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	awsClients ucpaws.Clients
}

// NewGetAWSOperationResults creates a new GetAWSOperationResults controller with the given options and AWS clients, and
// returns it without an error.
func NewGetAWSOperationResults(opts armrpc_controller.Options, awsClients ucpaws.Clients) (armrpc_controller.Controller, error) {
	return &GetAWSOperationResults{
		Operation:  armrpc_controller.NewOperation(opts, armrpc_controller.ResourceOptions[datamodel.AWSResource]{}),
		awsClients: awsClients,
	}, nil
}

// Run returns 202 while an AWS resource operation is in progress and 204 when it finished. UCP owns every create,
// update, and delete by alias, so their status is in UCP's data store.
//
// Transitional: an operation that UCP's data store does not have was started by POST /:put or /:delete, which do not
// track operations. Its ID is the Cloud Control request token, so Cloud Control is asked, as before resource records.
// This fallback is removed with those routes.
func (p *GetAWSOperationResults) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	serviceCtx := servicecontext.AWSRequestContextFromContext(ctx)
	region, errResponse := readRegionFromRequest(req.URL.Path, p.Options().PathBase)
	if errResponse != nil {
		return errResponse, nil
	}

	status, err := getOperationStatus(ctx, p.Options().StatusManager, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}
	terminal := false
	if status != nil {
		terminal = status.Status.IsTerminal()
	} else {
		response, err := p.awsClients.CloudControl.GetResourceRequestStatus(ctx, &cloudcontrol.GetResourceRequestStatusInput{
			RequestToken: aws.String(serviceCtx.ResourceID.Name()),
		}, CloudControlRegionOption(region))
		if ucpaws.IsAWSResourceNotFoundError(err) {
			return armrpc_rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
		} else if err != nil {
			return ucpaws.HandleAWSError(ctx, err)
		}
		terminal = isStatusTerminal(response)
	}

	if !terminal {
		headers := map[string]string{
			"Location":    req.URL.String(),
			"Retry-After": armrpcv1.DefaultRetryAfter,
		}
		return armrpc_rest.NewAsyncOperationResultResponse(headers), nil
	}
	return armrpc_rest.NewNoContentResponse(), nil
}

func isStatusTerminal(response *cloudcontrol.GetResourceRequestStatusOutput) bool {
	switch response.ProgressEvent.OperationStatus {
	case types.OperationStatusSuccess, types.OperationStatusCancelComplete, types.OperationStatusFailed:
		return true
	default:
		return false
	}
}
