// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package awsproxy

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/golang/mock/gomock"

	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
)

func Test_GetAWSResource(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	getResponseBody := map[string]interface{}{
		"RetentionPeriodHours": 178,
		"ShardCount":           3,
	}
	getResponseBodyBytes, err := json.Marshal(getResponseBody)
	require.NoError(t, err)

	testOptions := setupTest(t)
	testOptions.AWSClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudcontrol.GetResourceOutput{
			ResourceDescription: &types.ResourceDescription{
				Identifier: aws.String(testAWSResourceName),
				Properties: aws.String(string(getResponseBodyBytes)),
			},
		}, nil)

	awsController, err := NewGetAWSResource(ctrl.Options{
		AWSClient: testOptions.AWSClient,
		DB:        testOptions.StorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, testAWSSingleResourcePath, nil)

	require.NoError(t, err)
	actualResponse, err := awsController.Run(ctx, nil, request)

	expectedResponse := rest.NewOKResponse(map[string]interface{}{
		"id":   testAWSSingleResourcePath,
		"name": aws.String(testAWSResourceName),
		"type": testAWSResourceType,
		"properties": map[string]interface{}{
			"RetentionPeriodHours": float64(178),
			"ShardCount":           float64(3),
		},
	})

	require.NoError(t, err)
	require.Equal(t, expectedResponse, actualResponse)
}

func Test_GetAWSResource_NotFound(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	testOptions := setupTest(t)
	testOptions.AWSClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil, &types.ResourceNotFoundException{
			Message: aws.String("Resource not found"),
		})

	awsController, err := NewGetAWSResource(ctrl.Options{
		AWSClient: testOptions.AWSClient,
		DB:        testOptions.StorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, testAWSSingleResourcePath, nil)

	require.NoError(t, err)
	actualResponse, err := awsController.Run(ctx, nil, request)

	expectedResponse := rest.NewNotFoundResponse(testAWSSingleResourcePath)

	require.NoError(t, err)
	require.Equal(t, expectedResponse, actualResponse)
}

func Test_GetAWSResource_UnknownError(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	testOptions := setupTest(t)
	testOptions.AWSClient.EXPECT().GetResource(gomock.Any(), gomock.Any()).Return(nil, errors.New("something bad happened"))

	awsController, err := NewGetAWSResource(ctrl.Options{
		AWSClient: testOptions.AWSClient,
		DB:        testOptions.StorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, testAWSSingleResourcePath, nil)
	require.NoError(t, err)

	actualResponse, err := awsController.Run(ctx, nil, request)
	require.Error(t, err)

	require.Nil(t, actualResponse)
	require.Equal(t, "something bad happened", err.Error())
}

func Test_GetAWSResource_SmithyError(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	testOptions := setupTest(t)
	testOptions.AWSClient.EXPECT().GetResource(gomock.Any(), gomock.Any()).Return(nil, &smithy.OperationError{
		Err: &smithyhttp.ResponseError{
			Err: &smithy.GenericAPIError{
				Code:    "NotFound",
				Message: "Resource not found",
			},
		},
	})

	awsController, err := NewGetAWSResource(ctrl.Options{
		AWSClient: testOptions.AWSClient,
		DB:        testOptions.StorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, testAWSSingleResourcePath, nil)
	require.NoError(t, err)

	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	expectedResponse := rest.NewInternalServerErrorARMResponse(rest.ErrorResponse{
		Error: rest.ErrorDetails{
			Code:    "NotFound",
			Message: "Resource not found",
		},
	})

	require.Equal(t, expectedResponse, actualResponse)
}
