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

package resources

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ToAWSResourceType(t *testing.T) {
	expected := "AWS::S3::Bucket"
	resourceId := "/planes/aws/aws/accounts/123341234/regions/us-west-2/providers/AWS.S3/Bucket"

	id, err := Parse(resourceId)
	require.NoError(t, err)

	actual := ToAWSResourceType(id)

	require.Equal(t, expected, actual)
}
