// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package awsproxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"

	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
)

func Test_GetAWSOperationResults_TerminalStatus(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	testResource := CreateKinesisStreamTestResource(uuid.NewString())

	testOptions := setupTest(t)
	testOptions.AWSCloudControlClient.EXPECT().GetResourceRequestStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudcontrol.GetResourceRequestStatusOutput{
			ProgressEvent: &types.ProgressEvent{
				OperationStatus: types.OperationStatusSuccess,
				RequestToken:    aws.String(testAWSRequestToken),
			},
		}, nil)

	awsController, err := NewGetAWSOperationResults(
		ctrl.Options{
			StorageClient: testOptions.StorageClient,
		},
		AWSOptions{
			AWSCloudControlClient:   testOptions.AWSCloudControlClient,
			AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		},
	)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, testResource.OperationResultsPath, nil)

	require.NoError(t, err)
	actualResponse, err := awsController.Run(ctx, nil, request)

	expectedResponse := armrpc_rest.NewNoContentResponse()

	require.NoError(t, err)
	require.Equal(t, expectedResponse, actualResponse)
}

func Test_GetAWSOperationResults_NonTerminalStatus(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	testResource := CreateKinesisStreamTestResource(uuid.NewString())

	testOptions := setupTest(t)
	testOptions.AWSCloudControlClient.EXPECT().GetResourceRequestStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudcontrol.GetResourceRequestStatusOutput{
			ProgressEvent: &types.ProgressEvent{
				OperationStatus: types.OperationStatusSuccess,
				RequestToken:    aws.String(testAWSRequestToken),
			},
		}, nil)

	awsController, err := NewGetAWSOperationResults(
		ctrl.Options{
			StorageClient: testOptions.StorageClient,
		},
		AWSOptions{
			AWSCloudControlClient:   testOptions.AWSCloudControlClient,
			AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		},
	)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, testResource.OperationResultsPath, nil)

	require.NoError(t, err)
	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	err = actualResponse.Apply(ctx, w, request)
	require.NoError(t, err)

	res := w.Result()
	require.Equal(t, http.StatusNoContent, res.StatusCode)
}

func Test_IsStatusTerminal(t *testing.T) {
	require.True(t, isStatusTerminal(createResourceRequestStatusOutput(types.OperationStatusSuccess)))
	require.True(t, isStatusTerminal(createResourceRequestStatusOutput(types.OperationStatusCancelComplete)))
	require.True(t, isStatusTerminal(createResourceRequestStatusOutput(types.OperationStatusFailed)))
	require.False(t, isStatusTerminal(createResourceRequestStatusOutput(types.OperationStatusInProgress)))
	require.False(t, isStatusTerminal(createResourceRequestStatusOutput(types.OperationStatusPending)))
}

func createResourceRequestStatusOutput(status types.OperationStatus) *cloudcontrol.GetResourceRequestStatusOutput {
	return &cloudcontrol.GetResourceRequestStatusOutput{
		ProgressEvent: &types.ProgressEvent{
			OperationStatus: status,
		},
	}
}
