// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package awsproxy

import (
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/golang/mock/gomock"

	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	armrpc_v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
)

func Test_ListAWSResources(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	streamOneResourceName := "streamone"
	streamOneResponseBody := map[string]interface{}{
		"RetentionPeriodHours": 178,
		"ShardCount":           3,
	}
	streamOneResponseBodyBytes, err := json.Marshal(streamOneResponseBody)
	require.NoError(t, err)

	streamTwoResourceName := "streamtwo"
	streamTwoResponseBody := map[string]interface{}{
		"RetentionPeriodHours": 178,
		"ShardCount":           3,
	}
	streamTwoResponseBodyBytes, err := json.Marshal(streamTwoResponseBody)
	require.NoError(t, err)

	testOptions := setupTest(t)
	testOptions.AWSClient.EXPECT().ListResources(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudcontrol.ListResourcesOutput{
			ResourceDescriptions: []types.ResourceDescription{
				{
					Identifier: aws.String(streamOneResourceName),
					Properties: aws.String(string(streamOneResponseBodyBytes)),
				},
				{
					Identifier: aws.String(streamTwoResourceName),
					Properties: aws.String(string(streamTwoResponseBodyBytes)),
				},
			},
		}, nil)

	awsController, err := NewListAWSResources(ctrl.Options{
		AWSClient: testOptions.AWSClient,
		DB:        testOptions.StorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, testAWSResourceCollectionPath, nil)
	require.NoError(t, err)

	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	expectedResponse := armrpc_rest.NewOKResponse(map[string]interface{}{
		"value": []interface{}{
			map[string]interface{}{
				"id":   path.Join(testAWSResourceCollectionPath, streamOneResourceName),
				"name": aws.String(streamOneResourceName),
				"type": testAWSResourceType,
				"properties": map[string]interface{}{
					"RetentionPeriodHours": float64(178),
					"ShardCount":           float64(3),
				},
			},
			map[string]interface{}{
				"id":   path.Join(testAWSResourceCollectionPath, streamTwoResourceName),
				"name": aws.String(streamTwoResourceName),
				"type": testAWSResourceType,
				"properties": map[string]interface{}{
					"RetentionPeriodHours": float64(178),
					"ShardCount":           float64(3),
				},
			},
		},
	})

	require.Equal(t, expectedResponse, actualResponse)
}

func Test_ListAWSResourcesEmpty(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	testOptions := setupTest(t)
	testOptions.AWSClient.EXPECT().ListResources(gomock.Any(), gomock.Any()).Return(&cloudcontrol.ListResourcesOutput{}, nil)

	awsController, err := NewListAWSResources(ctrl.Options{
		AWSClient: testOptions.AWSClient,
		DB:        testOptions.StorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, testAWSResourceCollectionPath, nil)
	require.NoError(t, err)

	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	expectedResponse := armrpc_rest.NewOKResponse(map[string]interface{}{
		"value": []interface{}{},
	})

	require.Equal(t, expectedResponse, actualResponse)
}

func Test_ListAWSResource_UnknownError(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	testOptions := setupTest(t)
	testOptions.AWSClient.EXPECT().ListResources(gomock.Any(), gomock.Any()).Return(nil, errors.New("something bad happened"))

	awsController, err := NewListAWSResources(ctrl.Options{
		AWSClient: testOptions.AWSClient,
		DB:        testOptions.StorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, testAWSResourceCollectionPath, nil)
	require.NoError(t, err)

	actualResponse, err := awsController.Run(ctx, nil, request)
	require.Error(t, err)

	require.Nil(t, actualResponse)
	require.Equal(t, "something bad happened", err.Error())
}

func Test_ListAWSResource_SmithyError(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	testOptions := setupTest(t)
	testOptions.AWSClient.EXPECT().ListResources(gomock.Any(), gomock.Any()).Return(nil, &smithy.OperationError{
		Err: &smithyhttp.ResponseError{
			Err: &smithy.GenericAPIError{
				Code:    "NotFound",
				Message: "Resource not found",
			},
		},
	})

	awsController, err := NewListAWSResources(ctrl.Options{
		AWSClient: testOptions.AWSClient,
		DB:        testOptions.StorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, testAWSResourceCollectionPath, nil)
	require.NoError(t, err)

	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	expectedResponse := armrpc_rest.NewInternalServerErrorARMResponse(armrpc_v1.ErrorResponse{
		Error: armrpc_v1.ErrorDetails{
			Code:    "NotFound",
			Message: "Resource not found",
		},
	})

	require.Equal(t, expectedResponse, actualResponse)
}
