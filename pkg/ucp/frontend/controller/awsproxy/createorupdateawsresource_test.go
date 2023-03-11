// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package awsproxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/to"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
)

func Test_CreateAWSResource(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	testResource := CreateKinesisStreamTestResource(uuid.NewString())

	testOptions := setupTest(t)
	testOptions.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil, &types.ResourceNotFoundException{
			Message: aws.String("Resource not found"),
		})

	testOptions.AWSCloudControlClient.EXPECT().CreateResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudcontrol.CreateResourceOutput{
			ProgressEvent: &types.ProgressEvent{
				OperationStatus: types.OperationStatusSuccess,
				RequestToken:    to.Ptr(testAWSRequestToken),
			},
		}, nil)

	requestBody := map[string]any{
		"properties": map[string]any{
			"RetentionPeriodHours": 178,
			"ShardCount":           3,
		},
	}
	requestBodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	awsController, err := NewCreateOrUpdateAWSResource(ctrl.Options{
		AWSOptions: ctrl.AWSOptions{
			AWSCloudControlClient:   testOptions.AWSCloudControlClient,
			AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		},
		Options: armrpc_controller.Options{
			StorageClient: testOptions.StorageClient,
		},
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPut, testResource.SingleResourcePath, bytes.NewBuffer(requestBodyBytes))
	request.Host = testHost
	request.URL.Host = testHost
	request.URL.Scheme = testScheme

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

	require.NotNil(t, res.Header.Get("Location"))
	require.Equal(t, testResource.LocationHeader, res.Header.Get("Location"))

	require.NotNil(t, res.Header.Get("Azure-AsyncOperation"))
	require.Equal(t, testResource.AzureAsyncOpHeader, res.Header.Get("Azure-AsyncOperation"))
	defer res.Body.Close()

	expectedResponseObject := map[string]any{
		"id":   testResource.SingleResourcePath,
		"name": testResource.ResourceName,
		"type": testResource.ResourceType,
		"properties": map[string]any{
			"RetentionPeriodHours": float64(178),
			"ShardCount":           float64(3),
			"provisioningState":    "Provisioning",
		},
	}

	actualResponseObject := map[string]any{}
	err = json.Unmarshal(body, &actualResponseObject)
	require.NoError(t, err)

	require.Equal(t, expectedResponseObject, actualResponseObject)
}

func Test_UpdateAWSResource(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	testResource := CreateMemoryDBClusterTestResource(uuid.NewString())

	output := cloudformation.DescribeTypeOutput{
		TypeName: aws.String(testResource.AWSResourceType),
		Schema:   aws.String(testResource.Schema),
	}

	getResponseBody := map[string]any{
		"ClusterEndpoint": map[string]any{
			"Address": "test",
			"Port":    6379,
		},
		"Port":                6379,
		"ARN":                 "arn:aws:memorydb:us-west-2:123456789012:cluster:mycluster",
		"NumReplicasPerShard": 1,
	}
	getResponseBodyBytes, err := json.Marshal(getResponseBody)
	require.NoError(t, err)

	testOptions := setupTest(t)

	testOptions.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any()).Return(&output, nil)

	testOptions.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudcontrol.GetResourceOutput{
			ResourceDescription: &types.ResourceDescription{
				Properties: aws.String(string(getResponseBodyBytes)),
			},
		}, nil)

	testOptions.AWSCloudControlClient.EXPECT().UpdateResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudcontrol.UpdateResourceOutput{
			ProgressEvent: &types.ProgressEvent{
				OperationStatus: types.OperationStatusSuccess,
				RequestToken:    aws.String(testAWSRequestToken),
			},
		}, nil)

	requestBody := map[string]any{
		"properties": map[string]any{
			"Port":                6379,
			"NumReplicasPerShard": 0,
		},
	}
	requestBodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	awsController, err := NewCreateOrUpdateAWSResource(ctrl.Options{
		AWSOptions: ctrl.AWSOptions{
			AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
			AWSCloudControlClient:   testOptions.AWSCloudControlClient,
		},
		Options: armrpc_controller.Options{
			StorageClient: testOptions.StorageClient,
		},
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPut, testResource.SingleResourcePath, bytes.NewBuffer(requestBodyBytes))
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

	expectedResponseObject := map[string]any{
		"id":   testResource.SingleResourcePath,
		"name": testResource.ResourceName,
		"type": testResource.ResourceType,
		"properties": map[string]any{
			"ClusterEndpoint": map[string]any{
				"Address": "test",
				"Port":    float64(6379),
			},
			"Port":                float64(6379),
			"ARN":                 "arn:aws:memorydb:us-west-2:123456789012:cluster:mycluster",
			"NumReplicasPerShard": float64(0),
			"provisioningState":   "Provisioning",
		},
	}

	actualResponseObject := map[string]any{}
	err = json.Unmarshal(body, &actualResponseObject)
	require.NoError(t, err)

	require.Equal(t, expectedResponseObject, actualResponseObject)
}

func Test_UpdateNoChangesDoesNotCallUpdate(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	testResource := CreateKinesisStreamTestResource(uuid.NewString())

	output := cloudformation.DescribeTypeOutput{
		TypeName: aws.String(testResource.AWSResourceType),
		Schema:   aws.String(testResource.Schema),
	}

	getResponseBody := map[string]any{
		"RetentionPeriodHours": 178,
		"ShardCount":           3,
	}
	getResponseBodyBytes, err := json.Marshal(getResponseBody)
	require.NoError(t, err)

	testOptions := setupTest(t)

	testOptions.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any()).Return(&output, nil)

	testOptions.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudcontrol.GetResourceOutput{
			ResourceDescription: &types.ResourceDescription{
				Properties: to.Ptr(string(getResponseBodyBytes)),
			},
		}, nil)

	requestBody := map[string]any{
		"properties": map[string]any{
			"RetentionPeriodHours": 178,
			"ShardCount":           3,
		},
	}
	requestBodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	awsController, err := NewCreateOrUpdateAWSResource(ctrl.Options{
		AWSOptions: ctrl.AWSOptions{
			AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
			AWSCloudControlClient:   testOptions.AWSCloudControlClient,
		},
		Options: armrpc_controller.Options{
			StorageClient: testOptions.StorageClient,
		},
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPut, testResource.SingleResourcePath, bytes.NewBuffer(requestBodyBytes))
	require.NoError(t, err)

	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	err = actualResponse.Apply(ctx, w, request)
	require.NoError(t, err)

	res := w.Result()
	require.Equal(t, http.StatusOK, res.StatusCode)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	defer res.Body.Close()

	expectedResponseObject := map[string]any{
		"id":   testResource.SingleResourcePath,
		"name": testResource.ResourceName,
		"type": testResource.ResourceType,
		"properties": map[string]any{
			"RetentionPeriodHours": float64(178),
			"ShardCount":           float64(3),
			"provisioningState":    "Succeeded",
		},
	}

	actualResponseObject := map[string]any{}
	err = json.Unmarshal(body, &actualResponseObject)
	require.NoError(t, err)

	require.Equal(t, expectedResponseObject, actualResponseObject)
}
