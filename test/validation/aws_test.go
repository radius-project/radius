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
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/stretchr/testify/require"
)

func Test_IsAWSResourceNotFoundError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		resourceType string
		err          error
		expected     bool
	}{
		{
			name:         "typed ResourceNotFoundException is recognized for any resource type",
			resourceType: "AWS::S3::Bucket",
			err:          &types.ResourceNotFoundException{Message: aws.String("Resource not found")},
			expected:     true,
		},
		{
			name:         "log group InvalidRequestException: does not exist",
			resourceType: awsLogsLogGroupCloudControlType,
			err:          &types.InvalidRequestException{Message: aws.String("The specified log group does not exist.")},
			expected:     true,
		},
		{
			name:         "log group InvalidRequestException: cannot be found",
			resourceType: awsLogsLogGroupCloudControlType,
			err:          &types.InvalidRequestException{Message: aws.String("Log group cannot be found.")},
			expected:     true,
		},
		{
			name:         "log group InvalidRequestException with an unrelated message is not treated as not-found",
			resourceType: awsLogsLogGroupCloudControlType,
			err:          &types.InvalidRequestException{Message: aws.String("The request had invalid parameters.")},
			expected:     false,
		},
		{
			name:         "matching InvalidRequestException message is only recognized for AWS::Logs::LogGroup",
			resourceType: "AWS::S3::Bucket",
			err:          &types.InvalidRequestException{Message: aws.String("Log group cannot be found.")},
			expected:     false,
		},
		{
			name:         "generic error is not treated as not-found",
			resourceType: awsLogsLogGroupCloudControlType,
			err:          errors.New("some other error"),
			expected:     false,
		},
		{
			name:         "nil error is not treated as not-found",
			resourceType: awsLogsLogGroupCloudControlType,
			err:          nil,
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.expected, isAWSResourceNotFoundError(tt.resourceType, tt.err))
		})
	}
}
