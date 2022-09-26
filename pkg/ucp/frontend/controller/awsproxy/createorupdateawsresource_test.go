// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package awsproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/golang/mock/gomock"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
)

func Test_CreateAWSResource(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	mockAWSClient, mockStorageClient := setupMocks(t)
	mockAWSClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params *cloudcontrol.GetResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceOutput, error) {
		notFound := types.ResourceNotFoundException{
			Message: aws.String("Resource not found"),
		}
		return nil, &notFound
	})

	mockAWSClient.EXPECT().CreateResource(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params *cloudcontrol.CreateResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.CreateResourceOutput, error) {
		output := cloudcontrol.CreateResourceOutput{
			ProgressEvent: &types.ProgressEvent{
				OperationStatus: types.OperationStatusSuccess,
				RequestToken:    to.StringPtr(testAWSRequestToken),
			},
		}
		return &output, nil
	})

	requestBody := map[string]interface{}{
		"properties": map[string]interface{}{
			"RetentionPeriodHours": 178,
			"ShardCount":           3,
		},
	}
	requestBodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	awsController, err := NewCreateOrUpdateAWSResource(ctrl.Options{
		AWSClient: mockAWSClient,
		DB:        mockStorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPut, testAWSSingleResourcePath, bytes.NewBuffer(requestBodyBytes))
	require.NoError(t, err)

	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	err = actualResponse.Apply(ctx, w, request)
	require.NoError(t, err)

	res := w.Result()
	require.Equal(t, http.StatusCreated, res.StatusCode)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	defer res.Body.Close()

	expectedResponseObject := map[string]interface{}{
		"id":   testAWSSingleResourcePath,
		"name": testAWSResourceName,
		"type": testAWSResourceType,
		"properties": map[string]interface{}{
			"RetentionPeriodHours": float64(178),
			"ShardCount":           float64(3),
		},
	}

	actualResponseObject := map[string]interface{}{}
	err = json.Unmarshal(body, &actualResponseObject)
	require.NoError(t, err)

	require.Equal(t, expectedResponseObject, actualResponseObject)
}

func Test_UpdateAWSResource(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	getResponseBody := map[string]interface{}{
		"RetentionPeriodHours": 178,
		"ShardCount":           3,
	}
	getResponseBodyBytes, err := json.Marshal(getResponseBody)
	require.NoError(t, err)

	mockAWSClient, mockStorageClient := setupMocks(t)
	mockAWSClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params *cloudcontrol.GetResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceOutput, error) {
		output := cloudcontrol.GetResourceOutput{
			ResourceDescription: &types.ResourceDescription{
				Properties: to.StringPtr(string(getResponseBodyBytes)),
			},
		}
		return &output, nil
	})

	mockAWSClient.EXPECT().UpdateResource(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params *cloudcontrol.UpdateResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.UpdateResourceOutput, error) {
		output := cloudcontrol.UpdateResourceOutput{
			ProgressEvent: &types.ProgressEvent{
				OperationStatus: types.OperationStatusSuccess,
				RequestToken:    to.StringPtr(testAWSRequestToken),
			},
		}
		return &output, nil
	})

	requestBody := map[string]interface{}{
		"properties": map[string]interface{}{
			"RetentionPeriodHours": 180,
			"ShardCount":           4,
		},
	}
	requestBodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	awsController, err := NewCreateOrUpdateAWSResource(ctrl.Options{
		AWSClient: mockAWSClient,
		DB:        mockStorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPut, testAWSSingleResourcePath, bytes.NewBuffer(requestBodyBytes))
	require.NoError(t, err)

	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	err = actualResponse.Apply(ctx, w, request)
	require.NoError(t, err)

	res := w.Result()
	require.Equal(t, http.StatusCreated, res.StatusCode)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	defer res.Body.Close()

	expectedResponseObject := map[string]interface{}{
		"id":   testAWSSingleResourcePath,
		"name": testAWSResourceName,
		"type": testAWSResourceType,
		"properties": map[string]interface{}{
			"RetentionPeriodHours": float64(180),
			"ShardCount":           float64(4),
		},
	}

	actualResponseObject := map[string]interface{}{}
	err = json.Unmarshal(body, &actualResponseObject)
	require.NoError(t, err)

	require.Equal(t, expectedResponseObject, actualResponseObject)
}
