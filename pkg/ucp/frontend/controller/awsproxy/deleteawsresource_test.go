// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package awsproxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/golang/mock/gomock"

	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
)

func Test_DeleteAWSResource(t *testing.T) {
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
				Identifier: aws.String(testAWSResourceName),
				Properties: aws.String(string(getResponseBodyBytes)),
			},
		}
		return &output, nil
	})

	mockAWSClient.EXPECT().DeleteResource(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params *cloudcontrol.DeleteResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.DeleteResourceOutput, error) {
		output := cloudcontrol.DeleteResourceOutput{
			ProgressEvent: &types.ProgressEvent{
				OperationStatus: types.OperationStatusSuccess,
				RequestToken:    aws.String(testAWSRequestToken),
			},
		}
		return &output, nil
	})

	awsController, err := NewDeleteAWSResource(ctrl.Options{
		AWSClient: mockAWSClient,
		DB:        mockStorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodDelete, testAWSSingleResourcePath, nil)
	require.NoError(t, err)

	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	err = actualResponse.Apply(ctx, w, request)
	require.NoError(t, err)

	res := w.Result()
	require.Equal(t, http.StatusAccepted, res.StatusCode)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, []byte("{}"), body)
}

func Test_DeleteAWSResource_ResourceDoesNotExist(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	mockAWSClient, mockStorageClient := setupMocks(t)
	mockAWSClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params *cloudcontrol.GetResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceOutput, error) {
		notFound := types.ResourceNotFoundException{
			Message: aws.String("Resource not found"),
		}
		return nil, &notFound
	})

	awsController, err := NewDeleteAWSResource(ctrl.Options{
		AWSClient: mockAWSClient,
		DB:        mockStorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodDelete, testAWSSingleResourcePath, nil)
	require.NoError(t, err)

	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", testAWSSingleResourcePath, nil)
	w := httptest.NewRecorder()
	err = actualResponse.Apply(ctx, w, req)
	require.NoError(t, err)

	res := w.Result()
	require.Equal(t, http.StatusNoContent, res.StatusCode)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, []byte(""), body)
}
