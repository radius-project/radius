// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package awsproxy

import (
	"context"
	"fmt"
	http "net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	armrpcv1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	radrprest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/middleware"
	awserror "github.com/project-radius/radius/pkg/ucp/aws"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ ctrl.Controller = (*GetAWSOperationResults)(nil)

// GetAWSOperationResults is the controller implementation to delete AWS resource.
type GetAWSOperationResults struct {
	ctrl.BaseController
}

// NewGetAWSOperationResults creates a new GetAWSOperationResults.
func NewGetAWSOperationResults(opts ctrl.Options) (ctrl.Controller, error) {
	return &GetAWSOperationResults{ctrl.NewBaseController(opts)}, nil
}

func (p *GetAWSOperationResults) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	client := ctx.Value(middleware.AWSClientKey).(*cloudcontrol.Client)
	id := ctx.Value(middleware.AWSResourceID).(resources.ID)

	response, err := client.GetResourceRequestStatus(ctx, &cloudcontrol.GetResourceRequestStatusInput{
		RequestToken: aws.String(id.Name()),
	})
	if awserror.IsAWSResourceNotFound(err) {
		return rest.NewNotFoundResponse(id.String()), nil
	} else if err != nil {
		return awserror.HandleAWSError(err)
	}

	isTerminal := false
	switch response.ProgressEvent.OperationStatus {
	case types.OperationStatusSuccess:
		isTerminal = true
	case types.OperationStatusCancelComplete:
		isTerminal = true
	case types.OperationStatusFailed:
		isTerminal = true
	}

	logger := ucplog.GetLogger(ctx)
	logger.Info(fmt.Sprintf("SCHEME operationResults: Setting Location to %s", req.URL.String()))
	if !isTerminal {
		headers := map[string]string{
			"Location":    req.URL.String(),
			"Retry-After": armrpcv1.DefaultRetryAfter,
		}
		return radrprest.NewAsyncOperationResultResponse(headers), nil
	}

	return rest.NewNoContentResponse(), nil
}
