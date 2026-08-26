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
package validation

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"
)

func Test_isAWSResourceMissing(t *testing.T) {
	t.Parallel()

	// The CloudControl handler wraps the message from the downstream service, so reproduce that shape
	// rather than matching on the bare sentence.
	logGroupCannotBeFound := "AWS::Logs::LogGroup Handler returned status FAILED: Log group cannot be found. " +
		"(Service: CloudWatchLogs, Status Code: 400, Request ID: f57fca9e-6a8c-4adf-a7d0-65a25d8de2af) " +
		"(SDK Attempt Count: 1) (HandlerErrorCode: InvalidRequest)"
	logGroupDoesNotExist := "AWS::Logs::LogGroup Handler returned status FAILED: The specified log group does not exist. " +
		"(Service: CloudWatchLogs, Status Code: 400, Request ID: 5141a66f-fad4-4275-8120-a55d7486abac) " +
		"(SDK Attempt Count: 1) (HandlerErrorCode: InvalidRequest)"

	tests := []struct {
		name             string
		resourceTypeName string
		err              error
		expected         bool
	}{
		{
			name:             "nil error is not missing",
			resourceTypeName: awsLogGroupTypeName,
			err:              nil,
			expected:         false,
		},
		{
			name:             "typed ResourceNotFoundException is missing",
			resourceTypeName: awsLogGroupTypeName,
			err:              &types.ResourceNotFoundException{Message: aws.String("resource not found")},
			expected:         true,
		},
		{
			name:             "typed ResourceNotFoundException is missing for any resource type",
			resourceTypeName: "AWS::MemoryDB::Cluster",
			err:              &types.ResourceNotFoundException{Message: aws.String("resource not found")},
			expected:         true,
		},
		{
			name:             "log group cannot be found is missing",
			resourceTypeName: awsLogGroupTypeName,
			err:              &types.InvalidRequestException{Message: aws.String(logGroupCannotBeFound)},
			expected:         true,
		},
		{
			name:             "specified log group does not exist is missing",
			resourceTypeName: awsLogGroupTypeName,
			err:              &types.InvalidRequestException{Message: aws.String(logGroupDoesNotExist)},
			expected:         true,
		},
		{
			name:             "wrapped log group InvalidRequestException is missing",
			resourceTypeName: awsLogGroupTypeName,
			err: fmt.Errorf("operation error CloudControl: GetResource: %w",
				&types.InvalidRequestException{Message: aws.String(logGroupCannotBeFound)}),
			expected: true,
		},
		{
			name:             "unrelated InvalidRequestException is not missing",
			resourceTypeName: awsLogGroupTypeName,
			err:              &types.InvalidRequestException{Message: aws.String("Invalid patch document")},
			expected:         false,
		},
		{
			name:             "throttling error is not missing",
			resourceTypeName: awsLogGroupTypeName,
			err:              &types.ThrottlingException{Message: aws.String("rate exceeded")},
			expected:         false,
		},
		{
			name:             "log group message on a different resource type is not missing",
			resourceTypeName: "AWS::MemoryDB::Cluster",
			err:              &types.InvalidRequestException{Message: aws.String(logGroupCannotBeFound)},
			expected:         false,
		},
		{
			name:             "generic API error with matching message is not missing",
			resourceTypeName: awsLogGroupTypeName,
			err:              &smithy.GenericAPIError{Code: "InvalidRequestException", Message: logGroupCannotBeFound},
			expected:         false,
		},
		{
			name:             "arbitrary error is not missing",
			resourceTypeName: awsLogGroupTypeName,
			err:              errors.New("connection reset by peer"),
			expected:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.expected, isAWSResourceMissing(tt.resourceTypeName, tt.err))
		})
	}
}
