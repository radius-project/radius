// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package aws

import (
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/project-radius/radius/pkg/ucp/rest"
)

func IsAWSResourceNotFound(err error) bool {
	target := &types.ResourceNotFoundException{}
	return errors.As(err, &target)
}

func HandleAWSError(err error) (rest.Response, error) {
	operationErr := &smithy.OperationError{}
	if !errors.As(err, &operationErr) {
		return nil, err
	}

	httpErr := &smithyhttp.ResponseError{}
	if !errors.As(operationErr.Err, &httpErr) {
		return nil, err
	}

	var apiErr smithy.APIError
	if !errors.As(httpErr.Err, &apiErr) {
		return nil, err
	}

	e := rest.ErrorResponse{
		Error: rest.ErrorDetails{
			Code:    apiErr.ErrorCode(),
			Message: apiErr.ErrorMessage(),
		},
	}

	// We can't always trust apiErr.Fault :-/
	fault := apiErr.ErrorFault()
	if fault == smithy.FaultUnknown {
		switch apiErr.ErrorCode() {
		case "ValidationException":
			fault = smithy.FaultClient
		default:
			fault = smithy.FaultServer
		}
	}

	if fault == smithy.FaultClient {
		return rest.NewBadRequestARMResponse(e), nil
	}

	return rest.NewInternalServerErrorARMResponse(e), nil
}
