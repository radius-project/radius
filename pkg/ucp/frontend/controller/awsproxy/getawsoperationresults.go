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
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
)

var _ armrpc_controller.Controller = (*GetAWSOperationResults)(nil)

// GetAWSOperationResults is the controller implementation to get AWS resource operation results.
type GetAWSOperationResults struct {
	ctrl.BaseController
}

// NewGetAWSOperationResults creates a new GetAWSOperationResults.
func NewGetAWSOperationResults(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &GetAWSOperationResults{ctrl.NewBaseController(opts)}, nil
}

func (p *GetAWSOperationResults) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	cloudControlClient, _, _, id, err := ParseAWSRequest(ctx, p.Options, req)
	if err != nil {
		return nil, err
	}

	response, err := cloudControlClient.GetResourceRequestStatus(ctx, &cloudcontrol.GetResourceRequestStatusInput{
		RequestToken: aws.String(id.Name()),
	})
	if awsclient.IsAWSResourceNotFoundError(err) {
		return armrpc_rest.NewNotFoundResponse(id), nil
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
