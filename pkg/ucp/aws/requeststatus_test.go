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

package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func statusOutput(event *types.ProgressEvent) *cloudcontrol.GetResourceRequestStatusOutput {
	return &cloudcontrol.GetResourceRequestStatusOutput{ProgressEvent: event}
}

func Test_GetRequestStatus(t *testing.T) {
	tests := []struct {
		name  string
		event *types.ProgressEvent
		want  RequestStatus
	}{
		{"in progress", &types.ProgressEvent{OperationStatus: types.OperationStatusInProgress}, RequestStatus{}},
		{"success", &types.ProgressEvent{OperationStatus: types.OperationStatusSuccess, Identifier: aws.String("vpc-1")}, RequestStatus{Done: true, Succeeded: true, Identifier: "vpc-1"}},
		{"failed", &types.ProgressEvent{OperationStatus: types.OperationStatusFailed, ErrorCode: types.HandlerErrorCodeInvalidRequest, StatusMessage: aws.String("bad")}, RequestStatus{Done: true, Code: "InvalidRequest", Message: "bad"}},
		{"canceled", &types.ProgressEvent{OperationStatus: types.OperationStatusCancelComplete}, RequestStatus{Done: true}},
		{"delete of a missing resource", &types.ProgressEvent{OperationStatus: types.OperationStatusFailed, Operation: types.OperationDelete, ErrorCode: types.HandlerErrorCodeNotFound}, RequestStatus{Done: true, Succeeded: true, Code: "NotFound"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMockAWSCloudControlClient(gomock.NewController(t))
			client.EXPECT().GetResourceRequestStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(statusOutput(tt.event), nil)
			got, err := GetRequestStatus(t.Context(), client, "us-west-2", "token")
			require.NoError(t, err)
			require.Equal(t, tt.want, *got)
		})
	}

	client := NewMockAWSCloudControlClient(gomock.NewController(t))
	client.EXPECT().GetResourceRequestStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(statusOutput(nil), nil)
	_, err := GetRequestStatus(t.Context(), client, "us-west-2", "token")
	require.ErrorContains(t, err, "no status")
}

func Test_WaitForRequest(t *testing.T) {
	MinPollInterval, MaxPollInterval = time.Millisecond, 2*time.Millisecond
	client := NewMockAWSCloudControlClient(gomock.NewController(t))
	gomock.InOrder(
		client.EXPECT().GetResourceRequestStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(statusOutput(&types.ProgressEvent{OperationStatus: types.OperationStatusPending}), nil),
		client.EXPECT().GetResourceRequestStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(statusOutput(&types.ProgressEvent{OperationStatus: types.OperationStatusSuccess}), nil),
		client.EXPECT().GetResourceRequestStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(statusOutput(&types.ProgressEvent{OperationStatus: types.OperationStatusFailed, ErrorCode: types.HandlerErrorCodeAccessDenied}), nil),
		client.EXPECT().GetResourceRequestStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled")),
		client.EXPECT().GetResourceRequestStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(statusOutput(&types.ProgressEvent{OperationStatus: types.OperationStatusPending}), nil),
	)

	status, err := WaitForRequest(t.Context(), client, "us-west-2", "token")
	require.NoError(t, err)
	require.True(t, status.Succeeded)

	_, err = WaitForRequest(t.Context(), client, "us-west-2", "token")
	var failed *RequestStatus
	require.ErrorAs(t, err, &failed)
	require.True(t, failed.IsClientError())
	require.Contains(t, failed.Error(), "AccessDenied")

	_, err = WaitForRequest(t.Context(), client, "us-west-2", "token")
	require.ErrorContains(t, err, "throttled")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = WaitForRequest(ctx, client, "us-west-2", "token")
	require.ErrorContains(t, err, "stopped waiting")
}

func Test_IsClientError(t *testing.T) {
	require.True(t, (&RequestStatus{Code: "NotUpdatable"}).IsClientError())
	require.False(t, (&RequestStatus{Code: "InternalFailure"}).IsClientError())
	require.False(t, (&RequestStatus{Code: "Throttling"}).IsClientError())
}

func Test_IsClientFault(t *testing.T) {
	require.True(t, IsClientFault(&smithy.GenericAPIError{Code: "X", Fault: smithy.FaultClient}))
	require.True(t, IsClientFault(&smithy.GenericAPIError{Code: "ValidationException"}))
	require.False(t, IsClientFault(&smithy.GenericAPIError{Code: "X"}))
	require.False(t, IsClientFault(&smithy.GenericAPIError{Code: "X", Fault: smithy.FaultServer}))
	require.False(t, IsClientFault(errors.New("connection reset")))
}
