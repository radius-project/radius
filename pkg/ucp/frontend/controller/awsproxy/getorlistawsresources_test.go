// // ------------------------------------------------------------
// // Copyright (c) Microsoft Corporation.
// // Licensed under the MIT License.
// // ------------------------------------------------------------
package awsproxy

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/golang/mock/gomock"
	awstypes "github.com/project-radius/radius/pkg/ucp/aws"

	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func Test_GetAWSResource(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	mockClient := awstypes.NewMockAWSClient(mockCtrl)
	mockClient.EXPECT().GetResource(gomock.Any(), gomock.Any()).Return(&cloudcontrol.GetResourceOutput{
		TypeName: aws.String("AWS.Kinesis/Stream"),
		ResourceDescription: &types.ResourceDescription{
			Properties: aws.String(`{"InstanceId": "i-1234567890abcdef0"}`),
			Identifier: aws.String("streamz"),
		},
	}, nil)

	id, err := resources.ParseByMethod("/planes/aws/aws/accounts/11234/regions/us-west-2/providers/AWS.Kinesis/Stream/streamz", "GET")
	require.NoError(t, err)

	ctx = context.WithValue(ctx, AWSClientKey, mockClient)
	ctx = context.WithValue(ctx, AWSResourceTypeKey, "AWS.Kinesis/Stream")
	ctx = context.WithValue(ctx, AWSResourceID, id)

	awsController, err := NewGetOrListAWSResource(ctrl.Options{
		DB: mockStorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, "/", nil)

	require.NoError(t, err)
	actualResponse, err := awsController.Run(ctx, nil, request)

	expectedResponse := rest.NewOKResponse(map[string]interface{}{
		"id":         "/planes/aws/aws/accounts/11234/regions/us-west-2/providers/AWS.Kinesis/Stream/streamz",
		"name":       aws.String("streamz"),
		"type":       "AWS.Kinesis/Stream",
		"properties": map[string]interface{}{"InstanceId": "i-1234567890abcdef0"},
	})

	require.NoError(t, err)
	assert.DeepEqual(t, expectedResponse, actualResponse)
}

func Test_GetAWSResourceUnknownError(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	mockClient := awstypes.NewMockAWSClient(mockCtrl)
	mockClient.EXPECT().GetResource(gomock.Any(), gomock.Any()).Return(nil, errors.New("something bad happened"))

	id, err := resources.ParseByMethod("/planes/aws/aws/accounts/11234/regions/us-west-2/providers/AWS.Kinesis/Stream/streamz", "GET")
	require.NoError(t, err)

	ctx = context.WithValue(ctx, AWSClientKey, mockClient)
	ctx = context.WithValue(ctx, AWSResourceTypeKey, "AWS.Kinesis/Stream")
	ctx = context.WithValue(ctx, AWSResourceID, id)

	awsController, err := NewGetOrListAWSResource(ctrl.Options{
		DB: mockStorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, "/", nil)

	require.NoError(t, err)
	actualResponse, err := awsController.Run(ctx, nil, request)

	require.Error(t, err)
	require.Nil(t, actualResponse)
	require.Equal(t, "something bad happened", err.Error())
}

func Test_GetAWSResourceSmithyError(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	mockClient := awstypes.NewMockAWSClient(mockCtrl)
	mockClient.EXPECT().GetResource(gomock.Any(), gomock.Any()).Return(nil, &smithy.OperationError{
		Err: &smithyhttp.ResponseError{
			Err: &smithy.GenericAPIError{
				Code:    "NotFound",
				Message: "Resource not found",
			},
		},
	})

	id, err := resources.ParseByMethod("/planes/aws/aws/accounts/11234/regions/us-west-2/providers/AWS.Kinesis/Stream/streamz", "GET")
	require.NoError(t, err)

	ctx = context.WithValue(ctx, AWSClientKey, mockClient)
	ctx = context.WithValue(ctx, AWSResourceTypeKey, "AWS.Kinesis/Stream")
	ctx = context.WithValue(ctx, AWSResourceID, id)

	awsController, err := NewGetOrListAWSResource(ctrl.Options{
		DB: mockStorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, "/", nil)

	require.NoError(t, err)
	actualResponse, err := awsController.Run(ctx, nil, request)

	expectedResponse := rest.NewInternalServerErrorARMResponse(rest.ErrorResponse{
		Error: rest.ErrorDetails{
			Code:    "NotFound",
			Message: "Resource not found",
		},
	})

	require.NoError(t, err)
	assert.DeepEqual(t, expectedResponse, actualResponse)
}

func Test_ListAWSResources(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	mockClient := awstypes.NewMockAWSClient(mockCtrl)
	mockClient.EXPECT().ListResources(gomock.Any(), gomock.Any()).Return(&cloudcontrol.ListResourcesOutput{
		ResourceDescriptions: []types.ResourceDescription{
			{
				Properties: aws.String(`{"InstanceId": "i-1234567890abcdef0"}`),
				Identifier: aws.String("streamz"),
			},
			{
				Properties: aws.String(`{"InstanceId": "test123"}`),
				Identifier: aws.String("stream2"),
			},
		},
	}, nil)

	id, err := resources.ParseByMethod("/planes/aws/aws/accounts/11234/regions/us-west-2/providers/AWS.Kinesis/Stream", "GET")
	require.NoError(t, err)

	ctx = context.WithValue(ctx, AWSClientKey, mockClient)
	ctx = context.WithValue(ctx, AWSResourceTypeKey, "AWS.Kinesis/Stream")
	ctx = context.WithValue(ctx, AWSResourceID, id)

	awsController, err := NewGetOrListAWSResource(ctrl.Options{
		DB: mockStorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, "/", nil)

	require.NoError(t, err)
	actualResponse, err := awsController.Run(ctx, nil, request)

	expectedResponse := rest.NewOKResponse(map[string]interface{}{
		"value": []interface{}{
			map[string]interface{}{
				"id":         "/planes/aws/aws/accounts/11234/regions/us-west-2/providers/AWS.Kinesis/Stream/streamz",
				"name":       aws.String("streamz"),
				"type":       "AWS.Kinesis/Stream",
				"properties": map[string]interface{}{"InstanceId": "i-1234567890abcdef0"},
			},
			map[string]interface{}{
				"id":         "/planes/aws/aws/accounts/11234/regions/us-west-2/providers/AWS.Kinesis/Stream/stream2",
				"name":       aws.String("stream2"),
				"type":       "AWS.Kinesis/Stream",
				"properties": map[string]interface{}{"InstanceId": "test123"},
			},
		},
	})

	require.NoError(t, err)
	assert.DeepEqual(t, expectedResponse, actualResponse)
}

func Test_ListAWSResourcesEmpty(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	mockClient := awstypes.NewMockAWSClient(mockCtrl)
	mockClient.EXPECT().ListResources(gomock.Any(), gomock.Any()).Return(&cloudcontrol.ListResourcesOutput{}, nil)

	id, err := resources.ParseByMethod("/planes/aws/aws/accounts/11234/regions/us-west-2/providers/AWS.Kinesis/Stream", "GET")
	require.NoError(t, err)

	ctx = context.WithValue(ctx, AWSClientKey, mockClient)
	ctx = context.WithValue(ctx, AWSResourceTypeKey, "AWS.Kinesis/Stream")
	ctx = context.WithValue(ctx, AWSResourceID, id)

	awsController, err := NewGetOrListAWSResource(ctrl.Options{
		DB: mockStorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, "/", nil)

	require.NoError(t, err)
	actualResponse, err := awsController.Run(ctx, nil, request)

	expectedResponse := rest.NewOKResponse(map[string]interface{}{
		"value": []interface{}{},
	})

	require.NoError(t, err)
	assert.DeepEqual(t, expectedResponse, actualResponse)
}
