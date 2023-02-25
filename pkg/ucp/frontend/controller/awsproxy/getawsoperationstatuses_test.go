// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package awsproxy

import (
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/test/testutil"
	"github.com/stretchr/testify/require"
)

func Test_GetAWSOperationStatuses(t *testing.T) {
	testResource := CreateKinesisStreamTestResource(uuid.NewString())

	eventTime := time.Now()
	testOptions := setupTest(t)
	testOptions.AWSCloudControlClient.EXPECT().GetResourceRequestStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudcontrol.GetResourceRequestStatusOutput{
			ProgressEvent: &types.ProgressEvent{
				EventTime:       aws.Time(eventTime),
				OperationStatus: types.OperationStatusSuccess,
				RequestToken:    aws.String(testAWSRequestToken),
			},
		}, nil)

	awsController, err := NewGetAWSOperationStatuses(
		&AWSOptions{
			Options: ctrl.Options{
				StorageClient: testOptions.StorageClient,
			},
			AWSCloudControlClient:   testOptions.AWSCloudControlClient,
			AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		},
	)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, testResource.OperationStatusesPath, nil)
	require.NoError(t, err)
	ctx := testutil.ARMTestContextFromRequest(request)

	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	expectedResponse := armrpc_rest.NewOKResponse(v1.AsyncOperationStatus{
		Status:    v1.ProvisioningStateSucceeded,
		StartTime: eventTime,
	})

	require.Equal(t, expectedResponse, actualResponse)
}

func Test_GetAWSOperationStatuses_Failed(t *testing.T) {
	testResource := CreateKinesisStreamTestResource(uuid.NewString())

	eventTime := time.Now()
	errorCode := types.HandlerErrorCodeInternalFailure
	errorStatusMessage := "AsyncOperation Failed"

	testOptions := setupTest(t)
	testOptions.AWSCloudControlClient.EXPECT().GetResourceRequestStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudcontrol.GetResourceRequestStatusOutput{
			ProgressEvent: &types.ProgressEvent{
				EventTime:       aws.Time(eventTime),
				OperationStatus: types.OperationStatusFailed,
				ErrorCode:       errorCode,
				StatusMessage:   aws.String(errorStatusMessage),
				RequestToken:    aws.String(testAWSRequestToken),
			},
		}, nil)

	awsController, err := NewGetAWSOperationStatuses(
		&AWSOptions{
			Options: ctrl.Options{
				StorageClient: testOptions.StorageClient,
			},
			AWSCloudControlClient:   testOptions.AWSCloudControlClient,
			AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		},
	)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, testResource.OperationStatusesPath, nil)
	require.NoError(t, err)
	ctx := testutil.ARMTestContextFromRequest(request)

	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	expectedResponse := armrpc_rest.NewOKResponse(v1.AsyncOperationStatus{
		Status:    v1.ProvisioningStateFailed,
		StartTime: eventTime,
		Error: &v1.ErrorDetails{
			Code:    string(errorCode),
			Message: errorStatusMessage,
		},
	})

	require.Equal(t, expectedResponse, actualResponse)
}
