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
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/test/testutil"
	"github.com/stretchr/testify/require"
)

func Test_CreateAWSResourceWithPost(t *testing.T) {
	testOptions := setupTest(t)
	testResource := CreateMemoryDBClusterTestResource(uuid.NewString())

	testOptions.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any()).Return(
		&cloudformation.DescribeTypeOutput{
			TypeName: aws.String(testResource.AWSResourceType),
			Schema:   aws.String(testResource.Schema),
		}, nil)

	testOptions.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil, &types.ResourceNotFoundException{
			Message: aws.String("Resource not found"),
		})

	testOptions.AWSCloudControlClient.EXPECT().CreateResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudcontrol.CreateResourceOutput{
			ProgressEvent: &types.ProgressEvent{
				OperationStatus: types.OperationStatusSuccess,
				RequestToken:    aws.String(testAWSRequestToken),
				Identifier:      aws.String(testResource.ResourceName),
			},
		}, nil)

	requestBody := map[string]any{
		"properties": map[string]any{
			"ClusterName":         testResource.ResourceName,
			"Port":                6379,
			"NumReplicasPerShard": 1,
		},
	}
	requestBodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	awsController, err := NewCreateOrUpdateAWSResourceWithPost(
		&AWSOptions{
			Options: ctrl.Options{
				StorageClient: testOptions.StorageClient,
			},
			AWSCloudControlClient:   testOptions.AWSCloudControlClient,
			AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		},
	)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, testResource.CollectionPath, bytes.NewBuffer(requestBodyBytes))
	require.NoError(t, err)
	ctx := testutil.ARMTestContextFromRequest(request)

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
			"ClusterName":         testResource.ResourceName,
			"Port":                float64(6379),
			"NumReplicasPerShard": float64(1),
			"provisioningState":   "Provisioning",
		},
	}

	actualResponseObject := map[string]any{}
	err = json.Unmarshal(body, &actualResponseObject)
	require.NoError(t, err)

	require.Equal(t, expectedResponseObject, actualResponseObject)
}

func Test_UpdateAWSResourceWithPost(t *testing.T) {
	testResource := CreateMemoryDBClusterTestResource(uuid.NewString())

	output := cloudformation.DescribeTypeOutput{
		TypeName: aws.String(testResource.AWSResourceType),
		Schema:   aws.String(testResource.Schema),
	}

	testOptions := setupTest(t)
	testOptions.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any()).Return(&output, nil)

	getResponseBody := map[string]any{
		"ClusterName": testResource.ResourceName,
		"ClusterEndpoint": map[string]any{
			"Address": "test",
			"Port":    6379,
		},
		"Port":                6379,
		"ARN":                 testResource.ARN,
		"NumReplicasPerShard": 1,
	}
	getResponseBodyBytes, err := json.Marshal(getResponseBody)
	require.NoError(t, err)

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
				Identifier:      aws.String(testResource.ResourceName),
			},
		}, nil)

	requestBody := map[string]any{
		"properties": map[string]any{
			"ClusterName":         testResource.ResourceName,
			"Port":                6379,
			"NumReplicasPerShard": 0,
		},
	}
	requestBodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	awsController, err := NewCreateOrUpdateAWSResourceWithPost(
		&AWSOptions{
			Options: ctrl.Options{
				StorageClient: testOptions.StorageClient,
			},
			AWSCloudControlClient:   testOptions.AWSCloudControlClient,
			AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		},
	)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, testResource.CollectionPath, bytes.NewBuffer(requestBodyBytes))
	require.NoError(t, err)
	ctx := testutil.ARMTestContextFromRequest(request)

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
			"ClusterName": testResource.ResourceName,
			"ClusterEndpoint": map[string]any{
				"Address": "test",
				"Port":    float64(6379),
			},
			"Port":                float64(6379),
			"ARN":                 testResource.ARN,
			"NumReplicasPerShard": float64(0),
			"provisioningState":   "Provisioning",
		},
	}

	actualResponseObject := map[string]any{}
	err = json.Unmarshal(body, &actualResponseObject)
	require.NoError(t, err)

	require.Equal(t, expectedResponseObject, actualResponseObject)
}

func Test_UpdateAWSResourceWithPost_NoChangesNoops(t *testing.T) {
	testResource := CreateMemoryDBClusterTestResource(uuid.NewString())

	output := cloudformation.DescribeTypeOutput{
		TypeName: aws.String(testResource.AWSResourceType),
		Schema:   aws.String(testResource.Schema),
	}

	testOptions := setupTest(t)
	testOptions.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any()).Return(&output, nil)

	getResponseBody := map[string]any{
		"ClusterName": testResource.ResourceName,
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

	testOptions.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudcontrol.GetResourceOutput{
			ResourceDescription: &types.ResourceDescription{
				Properties: aws.String(string(getResponseBodyBytes)),
			},
		}, nil)

	requestBody := map[string]any{
		"properties": map[string]any{
			"ClusterName":         testResource.ResourceName,
			"Port":                6379,
			"NumReplicasPerShard": 1,
		},
	}
	requestBodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	awsController, err := NewCreateOrUpdateAWSResourceWithPost(
		&AWSOptions{
			Options: ctrl.Options{
				StorageClient: testOptions.StorageClient,
			},
			AWSCloudControlClient:   testOptions.AWSCloudControlClient,
			AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		},
	)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/AWS.MemoryDB/Cluster", bytes.NewBuffer(requestBodyBytes))
	require.NoError(t, err)
	ctx := testutil.ARMTestContextFromRequest(request)

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
			"ClusterName": testResource.ResourceName,
			"ClusterEndpoint": map[string]any{
				"Address": "test",
				"Port":    float64(6379),
			},
			"Port":                float64(6379),
			"ARN":                 "arn:aws:memorydb:us-west-2:123456789012:cluster:mycluster",
			"NumReplicasPerShard": float64(1),
			"provisioningState":   "Succeeded",
		},
	}

	actualResponseObject := map[string]any{}
	err = json.Unmarshal(body, &actualResponseObject)
	require.NoError(t, err)

	require.Equal(t, expectedResponseObject, actualResponseObject)
}

func Test_CreateAWSResourceWithPost_NoPrimaryIdentifierAvailable(t *testing.T) {
	testResource := CreateRedshiftEndpointAuthorizationTestResource(uuid.NewString())
	clusterIdentifierValue := "abc"
	accountValue := "xyz"
	multiIdentifierResourceID := clusterIdentifierValue + "|" + accountValue

	output := cloudformation.DescribeTypeOutput{
		TypeName: aws.String(testResource.AWSResourceType),
		Schema:   aws.String(testResource.Schema),
	}

	testOptions := setupTest(t)
	testOptions.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any()).Return(&output, nil)

	testOptions.AWSCloudControlClient.EXPECT().CreateResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudcontrol.CreateResourceOutput{
			ProgressEvent: &types.ProgressEvent{
				OperationStatus: types.OperationStatusSuccess,
				RequestToken:    aws.String(testAWSRequestToken),
				Identifier:      aws.String(multiIdentifierResourceID),
			},
		}, nil)

	requestBody := map[string]any{
		"properties": map[string]any{
			"ClusterIdentifier": clusterIdentifierValue,
		},
	}
	requestBodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	awsController, err := NewCreateOrUpdateAWSResourceWithPost(
		&AWSOptions{
			Options: ctrl.Options{
				StorageClient: testOptions.StorageClient,
			},
			AWSCloudControlClient:   testOptions.AWSCloudControlClient,
			AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		},
	)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, testResource.CollectionPath+"/:put", bytes.NewBuffer(requestBodyBytes))
	require.NoError(t, err)
	ctx := testutil.ARMTestContextFromRequest(request)

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

	id, err := resources.Parse(testResource.CollectionPath)
	require.NoError(t, err)
	rID := computeResourceID(id, multiIdentifierResourceID)
	expectedResponseObject := map[string]any{
		"id":   rID,
		"name": multiIdentifierResourceID,
		"type": testResource.ResourceType,
		"properties": map[string]any{
			"ClusterIdentifier": clusterIdentifierValue,
			"provisioningState": "Provisioning",
		},
	}

	actualResponseObject := map[string]any{}
	err = json.Unmarshal(body, &actualResponseObject)
	require.NoError(t, err)

	require.Equal(t, expectedResponseObject, actualResponseObject)
}

func Test_CreateAWSResourceWithPost_MultiIdentifier(t *testing.T) {
	testResource := CreateRedshiftEndpointAuthorizationTestResource(uuid.NewString())
	clusterIdentifierValue := "abc"
	accountValue := "xyz"
	multiIdentifierResourceID := clusterIdentifierValue + "|" + accountValue

	output := cloudformation.DescribeTypeOutput{
		TypeName: aws.String(testResource.AWSResourceType),
		Schema:   aws.String(testResource.Schema),
	}

	testOptions := setupTest(t)
	testOptions.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any()).Return(&output, nil)

	testOptions.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil, &types.ResourceNotFoundException{
			Message: aws.String("Resource not found"),
		})

	testOptions.AWSCloudControlClient.EXPECT().CreateResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudcontrol.CreateResourceOutput{
			ProgressEvent: &types.ProgressEvent{
				OperationStatus: types.OperationStatusSuccess,
				RequestToken:    aws.String(testAWSRequestToken),
				Identifier:      aws.String(multiIdentifierResourceID),
			},
		}, nil)

	requestBody := map[string]any{
		"properties": map[string]any{
			"ClusterIdentifier": clusterIdentifierValue,
			"Account":           accountValue,
		},
	}
	requestBodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	awsController, err := NewCreateOrUpdateAWSResourceWithPost(
		&AWSOptions{
			Options: ctrl.Options{
				StorageClient: testOptions.StorageClient,
			},
			AWSCloudControlClient:   testOptions.AWSCloudControlClient,
			AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		},
	)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, testResource.CollectionPath+"/:put", bytes.NewBuffer(requestBodyBytes))
	require.NoError(t, err)
	ctx := testutil.ARMTestContextFromRequest(request)

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

	id, err := resources.Parse(testResource.CollectionPath)
	require.NoError(t, err)
	rID := computeResourceID(id, multiIdentifierResourceID)
	expectedResponseObject := map[string]any{
		"id":   rID,
		"name": multiIdentifierResourceID,
		"type": testResource.ResourceType,
		"properties": map[string]any{
			"ClusterIdentifier": clusterIdentifierValue,
			"Account":           accountValue,
			"provisioningState": "Provisioning",
		},
	}

	actualResponseObject := map[string]any{}
	err = json.Unmarshal(body, &actualResponseObject)
	require.NoError(t, err)

	require.Equal(t, expectedResponseObject, actualResponseObject)
}

func Test_UpdateAWSResourceWithPost_MultiIdentifier(t *testing.T) {
	testResource := CreateRedshiftEndpointAuthorizationTestResource(uuid.NewString())
	clusterIdentifierValue := "abc"
	accountValue := "xyz"
	multiIdentifierResourceID := clusterIdentifierValue + "|" + accountValue

	output := cloudformation.DescribeTypeOutput{
		TypeName: aws.String(testResource.AWSResourceType),
		Schema:   aws.String(testResource.Schema),
	}

	testOptions := setupTest(t)
	testOptions.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any()).Return(&output, nil)

	getResponseBody := map[string]any{
		"ClusterIdentifier": clusterIdentifierValue,
		"Account":           accountValue,
	}
	getResponseBodyBytes, err := json.Marshal(getResponseBody)
	require.NoError(t, err)

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
				Identifier:      aws.String(multiIdentifierResourceID),
			},
		}, nil)

	requestBody := map[string]any{
		"properties": map[string]any{
			"ClusterIdentifier": clusterIdentifierValue,
			"Account":           accountValue,
			"EndpointCount":     2,
		},
	}
	requestBodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	awsController, err := NewCreateOrUpdateAWSResourceWithPost(
		&AWSOptions{
			Options: ctrl.Options{
				StorageClient: testOptions.StorageClient,
			},
			AWSCloudControlClient:   testOptions.AWSCloudControlClient,
			AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		},
	)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, testResource.CollectionPath, bytes.NewBuffer(requestBodyBytes))
	require.NoError(t, err)
	ctx := testutil.ARMTestContextFromRequest(request)

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

	id, err := resources.Parse(testResource.CollectionPath)
	require.NoError(t, err)
	rID := computeResourceID(id, multiIdentifierResourceID)
	expectedResponseObject := map[string]any{
		"id":   rID,
		"name": multiIdentifierResourceID,
		"type": testResource.ResourceType,
		"properties": map[string]any{
			"ClusterIdentifier": "abc",
			"Account":           "xyz",
			"EndpointCount":     float64(2),
			"provisioningState": "Provisioning",
		},
	}

	actualResponseObject := map[string]any{}
	err = json.Unmarshal(body, &actualResponseObject)
	require.NoError(t, err)

	require.Equal(t, expectedResponseObject, actualResponseObject)
}
