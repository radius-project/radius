// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package awsproxy

import (
	"context"
	"encoding/json"
	"net/http"
	"path"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/golang/mock/gomock"

	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/rest"
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

	mockAWSClient, mockStorageClient := setupMocks(t)
	mockAWSClient.EXPECT().ListResources(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params *cloudcontrol.ListResourcesInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.ListResourcesOutput, error) {
		output := cloudcontrol.ListResourcesOutput{
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
		}
		return &output, nil
	})

	awsController, err := NewListAWSResources(ctrl.Options{
		AWSClient: mockAWSClient,
		DB:        mockStorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, testAWSResourceCollectionPath, nil)
	require.NoError(t, err)

	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	expectedResponse := rest.NewOKResponse(map[string]interface{}{
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

	mockAWSClient, mockStorageClient := setupMocks(t)
	mockAWSClient.EXPECT().ListResources(gomock.Any(), gomock.Any()).Return(&cloudcontrol.ListResourcesOutput{}, nil)

	awsController, err := NewListAWSResources(ctrl.Options{
		AWSClient: mockAWSClient,
		DB:        mockStorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, testAWSResourceCollectionPath, nil)
	require.NoError(t, err)

	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	expectedResponse := rest.NewOKResponse(map[string]interface{}{
		"value": []interface{}{},
	})

	require.Equal(t, expectedResponse, actualResponse)
}
