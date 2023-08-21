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
package aws

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	armrpc_v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
)

// HandleAWSError takes in an error and returns an ARMResponse and an error. It checks the error type and returns an
// appropriate response based on the error code and message. If the error is a client fault, it returns a
// BadRequestARMResponse, otherwise it returns an InternalServerErrorARMResponse.
func HandleAWSError(err error) (armrpc_rest.Response, error) {
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

	e := armrpc_v1.ErrorResponse{
		Error: armrpc_v1.ErrorDetails{
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
		return armrpc_rest.NewBadRequestARMResponse(e), nil
	}

	return armrpc_rest.NewInternalServerErrorARMResponse(e), nil
}

// Checks if the given error is an AWS ResourceNotFoundException error. If so, it returns true, otherwise false.
func IsAWSResourceNotFoundError(err error) bool {
	target := &types.ResourceNotFoundException{}
	return errors.As(err, &target)
}

// AWSMissingPropertyError is an error type to be returned when the call to UCP CreateWithPost
// is missing values for one of the expected primary identifier properties
type AWSMissingPropertyError struct {
	PropertyName string
}

// Is checks if the target error is of type AWSMissingPropertyError and returns a boolean value indicating the result.
func (e *AWSMissingPropertyError) Is(target error) bool {
	_, ok := target.(*AWSMissingPropertyError)
	return ok
}

// Error returns a string containing the name of the missing property for an AWSMissingPropertyError.
func (e *AWSMissingPropertyError) Error() string {
	return fmt.Sprintf("mandatory property %s is missing", e.PropertyName)
}
