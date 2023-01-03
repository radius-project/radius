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
	"github.com/google/uuid"

	armrpc_v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
)

func Test_GetAWSResource(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	testResource := CreateKinesisStreamTestResource(uuid.NewString())

	getResponseBody := map[string]any{
		"RetentionPeriodHours": 178,
		"ShardCount":           3,
	}
	getResponseBodyBytes, err := json.Marshal(getResponseBody)
	require.NoError(t, err)

	testOptions := setupTest(t)
	testOptions.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudcontrol.GetResourceOutput{
			ResourceDescription: &types.ResourceDescription{
				Identifier: aws.String(testResource.ResourceName),
				Properties: aws.String(string(getResponseBodyBytes)),
			},
		}, nil)

	awsController, err := NewGetAWSResource(ctrl.Options{
		AWSCloudControlClient: testOptions.AWSCloudControlClient,
		DB:                    testOptions.StorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, testResource.SingleResourcePath, nil)

	require.NoError(t, err)
	actualResponse, err := awsController.Run(ctx, nil, request)

	expectedResponse := armrpc_rest.NewOKResponse(map[string]any{
		"id":   testResource.SingleResourcePath,
		"name": aws.String(testResource.ResourceName),
		"type": testResource.ResourceType,
		"properties": map[string]any{
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

	testResource := CreateKinesisStreamTestResource(uuid.NewString())

	testOptions := setupTest(t)
	testOptions.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil, &types.ResourceNotFoundException{
			Message: aws.String("Resource not found"),
		})

	awsController, err := NewGetAWSResource(ctrl.Options{
		AWSCloudControlClient: testOptions.AWSCloudControlClient,
		DB:                    testOptions.StorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, testResource.SingleResourcePath, nil)

	require.NoError(t, err)
	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	id, err := resources.ParseResource(testResource.SingleResourcePath)
	require.NoError(t, err)

	expectedResponse := armrpc_rest.NewNotFoundResponse(id)
	require.Equal(t, expectedResponse, actualResponse)
}

func Test_GetAWSResource_UnknownError(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	testResource := CreateKinesisStreamTestResource(uuid.NewString())

	testOptions := setupTest(t)
	testOptions.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any()).Return(nil, errors.New("something bad happened"))

	awsController, err := NewGetAWSResource(ctrl.Options{
		AWSCloudControlClient: testOptions.AWSCloudControlClient,
		DB:                    testOptions.StorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, testResource.SingleResourcePath, nil)
	require.NoError(t, err)

	actualResponse, err := awsController.Run(ctx, nil, request)
	require.Error(t, err)

	require.Nil(t, actualResponse)
	require.Equal(t, "something bad happened", err.Error())
}

func Test_GetAWSResource_SmithyError(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	testResource := CreateKinesisStreamTestResource(uuid.NewString())

	testOptions := setupTest(t)
	testOptions.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any()).Return(nil, &smithy.OperationError{
		Err: &smithyhttp.ResponseError{
			Err: &smithy.GenericAPIError{
				Code:    "NotFound",
				Message: "Resource not found",
			},
		},
	})

	awsController, err := NewGetAWSResource(ctrl.Options{
		AWSCloudControlClient: testOptions.AWSCloudControlClient,
		DB:                    testOptions.StorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, testResource.SingleResourcePath, nil)
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
