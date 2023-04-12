// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
