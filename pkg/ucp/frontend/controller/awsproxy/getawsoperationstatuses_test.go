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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
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

	awsClients := ucp_aws.Clients{
		CloudControl:   testOptions.AWSCloudControlClient,
		CloudFormation: testOptions.AWSCloudFormationClient,
	}
	awsController, err := NewGetAWSOperationStatuses(armrpc_controller.Options{StorageClient: testOptions.StorageClient}, awsClients)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, testResource.OperationStatusesPath, nil)
	require.NoError(t, err)

	ctx := rpctest.NewARMRequestContext(request)
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

	awsClients := ucp_aws.Clients{
		CloudControl:   testOptions.AWSCloudControlClient,
		CloudFormation: testOptions.AWSCloudFormationClient,
	}
	awsController, err := NewGetAWSOperationStatuses(armrpc_controller.Options{StorageClient: testOptions.StorageClient}, awsClients)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, testResource.OperationStatusesPath, nil)
	require.NoError(t, err)

	ctx := rpctest.NewARMRequestContext(request)
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
