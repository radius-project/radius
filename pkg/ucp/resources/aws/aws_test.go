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
	"testing"

	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

func Test_ToAWSResourceType(t *testing.T) {
	expected := "AWS::S3::Bucket"
	resourceId := "/planes/aws/aws/accounts/123341234/regions/us-west-2/providers/AWS.S3/Bucket"

	id, err := resources.Parse(resourceId)
	require.NoError(t, err)

	actual := ToAWSResourceType(id)

	require.Equal(t, expected, actual)
}

func Test_ToUCPResourceID(t *testing.T) {
	t.Run("arn format 1", func(t *testing.T) {
		arn := "arn:aws:ec2:us-east-2:179022619019:subnet/subnet-0ddfaa93733f98002"
		expectedID := "/planes/aws/aws/accounts/179022619019/regions/us-east-2/providers/AWS.ec2/subnet/subnet-0ddfaa93733f98002"
		ucpID, err := ToUCPResourceID(arn)
		require.NoError(t, err)
		require.Equal(t, ucpID, expectedID)
	})

	t.Run("arn format 2", func(t *testing.T) {
		arn := "arn:aws:ec2:us-east-2:179022619019:subnet:subnet-0ddfaa93733f98002"
		expectedID := "/planes/aws/aws/accounts/179022619019/regions/us-east-2/providers/AWS.ec2/subnet/subnet-0ddfaa93733f98002"
		ucpID, err := ToUCPResourceID(arn)
		require.NoError(t, err)
		require.Equal(t, ucpID, expectedID)
	})

	t.Run("arn format 3", func(t *testing.T) {
		arn := "arn:aws:ec2:us-east-2:179022619019:subnet-0ddfaa93733f98002"
		expectedID := "/planes/aws/aws/accounts/179022619019/regions/us-east-2/providers/AWS.ec2/subnet-0ddfaa93733f98002"
		ucpID, err := ToUCPResourceID(arn)
		require.NoError(t, err)
		require.Equal(t, expectedID, ucpID)
	})

	t.Run("invalid arn", func(t *testing.T) {
		arn := "arn:aws:ec2:us-east-2:179022619019"
		_, err := ToUCPResourceID(arn)
		require.EqualError(t, err, "\"arn:aws:ec2:us-east-2:179022619019\" is not a valid ARN")
	})
}
