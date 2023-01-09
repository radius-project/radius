// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rediscaches

import (
	"context"
	"fmt"
	"testing"

	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/stretchr/testify/require"
)

func Test_Transform_Success(t *testing.T) {
	ctx := context.Background()
	redisTransformer := AzureTransformer{}

	testComputedValues := map[string]any{
		renderers.Host: "test-hostname",
		renderers.Port: "1234",
	}
	testPrimaryKey := "test-password"
	expectedConnectionString := "test-hostname:1234,password=test-password,ssl=True,abortConnect=False"

	connectionString, err := redisTransformer.Transform(ctx, testComputedValues, testPrimaryKey)
	require.NoError(t, err)
	require.Equal(t, expectedConnectionString, connectionString)
}

func Test_Transform_Error(t *testing.T) {
	ctx := context.Background()
	redisTransformer := AzureTransformer{}

	testCases := []struct {
		description        string
		primaryKey         any
		computedValues     map[string]any
		expectedErrMessage string
	}{
		{
			"Invalid primary key format",
			1234,
			map[string]any{
				renderers.Host: "test-hostname",
				renderers.Port: "1234",
			},
			"expected the access key to be a string",
		},
		{
			"Missing hostname",
			"test-password",
			map[string]any{
				renderers.Port: "1234",
			},
			"hostname is required to build Redis connection string",
		},
		{
			"Missing port",
			"test-password",
			map[string]any{
				renderers.Host: "test-hostname",
			},
			"port is required to build Redis connection string",
		},
	}

	for _, testCase := range testCases {
		t.Run(fmt.Sprint(testCase.description), func(t *testing.T) {
			_, err := redisTransformer.Transform(ctx, testCase.computedValues, testCase.primaryKey)
			require.Error(t, err)
			require.Equal(t, testCase.expectedErrMessage, err.Error())
		})
	}
}
