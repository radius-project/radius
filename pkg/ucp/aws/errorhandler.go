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

// # Function Explanation
// 
//	HandleAWSError is a function that handles errors returned from AWS services. It checks the error type and returns an 
//	appropriate response with an error code and message. It also sets the fault type to either Client or Server depending on
//	 the error code. This helps the callers of this function to handle the errors appropriately.
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

// # Function Explanation
// 
//	IsAWSResourceNotFoundError checks if the given error is an AWS ResourceNotFoundException and returns a boolean value 
//	accordingly. It can be used by callers of this function to determine if the error is a ResourceNotFoundException and 
//	take appropriate action.
func IsAWSResourceNotFoundError(err error) bool {
	target := &types.ResourceNotFoundException{}
	return errors.As(err, &target)
}

// AWSMissingPropertyError is an error type to be returned when the call to UCP CreateWithPost
// is missing values for one of the expected primary identifier properties
type AWSMissingPropertyError struct {
	PropertyName string
}

// # Function Explanation
// 
//	AWSMissingPropertyError's Is() method checks if the given error is an instance of AWSMissingPropertyError and returns a 
//	boolean value accordingly, allowing callers to handle the error appropriately.
func (e *AWSMissingPropertyError) Is(target error) bool {
	_, ok := target.(*AWSMissingPropertyError)
	return ok
}

// # Function Explanation
// 
//	AWSMissingPropertyError is an error type that is returned when a mandatory property is missing from a request. It 
//	provides a helpful error message to the caller, informing them of the missing property.
func (e *AWSMissingPropertyError) Error() string {
	return fmt.Sprintf("mandatory property %s is missing", e.PropertyName)
}
