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
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
)

var _ ctrl.Controller = (*GetAWSOperationResults)(nil)

// GetAWSOperationResults is the controller implementation to get AWS resource operation results.
type GetAWSOperationResults struct {
	ctrl.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	AWSOptions
}

// NewGetAWSOperationResults creates a new GetAWSOperationResults.
func NewGetAWSOperationResults(opts ctrl.Options, awsOpts AWSOptions) (ctrl.Controller, error) {
	return &GetAWSOperationResults{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.AWSResource]{},
		),
		awsOpts,
	}, nil
}

func (p *GetAWSOperationResults) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
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
