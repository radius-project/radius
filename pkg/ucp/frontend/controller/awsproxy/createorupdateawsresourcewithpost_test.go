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
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
)

func Test_CreateAWSResourceWithPost(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	testOptions := setupTest(t)

	primaryIdentifiers := map[string]interface{}{
		"primaryIdentifier": []interface{}{
			"/properties/Name",
		},
	}
	serialized, err := json.Marshal(primaryIdentifiers)
	require.NoError(t, err)
	output := cloudformation.DescribeTypeOutput{
		TypeName: aws.String("AWS::Kinesis::Stream"),
		Schema:   aws.String(string(serialized)),
	}

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
			},
		}, nil)

	requestBody := map[string]interface{}{
		"properties": map[string]interface{}{
			"Name":                 testAWSResourceName,
			"RetentionPeriodHours": 178,
			"ShardCount":           3,
		},
	}
	requestBodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	awsController, err := NewCreateOrUpdateAWSResourceWithPost(ctrl.Options{
		AWSCloudControlClient:   testOptions.AWSCloudControlClient,
		AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		DB:                      testOptions.StorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, testAWSResourceCollectionPath+"/:put", bytes.NewBuffer(requestBodyBytes))
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
			"Name":                 testAWSResourceName,
			"RetentionPeriodHours": float64(178),
			"ShardCount":           float64(3),
			"provisioningState":    "Provisioning",
		},
	}

	actualResponseObject := map[string]interface{}{}
	err = json.Unmarshal(body, &actualResponseObject)
	require.NoError(t, err)

	require.Equal(t, expectedResponseObject, actualResponseObject)
}

func Test_UpdateAWSResourceWithPost(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	testOptions := setupTest(t)
	primaryIdentifiers := map[string]interface{}{
		"primaryIdentifier": []interface{}{
			"/properties/Name",
		},
	}
	serialized, err := json.Marshal(primaryIdentifiers)
	require.NoError(t, err)
	output := cloudformation.DescribeTypeOutput{
		TypeName: aws.String("AWS::Kinesis::Stream"),
		Schema:   aws.String(string(serialized)),
	}

	testOptions.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any()).Return(&output, nil)

	getResponseBody := map[string]interface{}{
		"Name":                 testAWSResourceName,
		"RetentionPeriodHours": 178,
		"ShardCount":           3,
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
			},
		}, nil)

	requestBody := map[string]interface{}{
		"properties": map[string]interface{}{
			"Name":                 testAWSResourceName,
			"RetentionPeriodHours": 180,
			"ShardCount":           4,
		},
	}
	requestBodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	awsController, err := NewCreateOrUpdateAWSResourceWithPost(ctrl.Options{
		AWSCloudControlClient:   testOptions.AWSCloudControlClient,
		AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		DB:                      testOptions.StorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, testAWSResourceCollectionPath, bytes.NewBuffer(requestBodyBytes))
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
			"Name":                 testAWSResourceName,
			"RetentionPeriodHours": float64(180),
			"ShardCount":           float64(4),
			"provisioningState":    "Provisioning",
		},
	}

	actualResponseObject := map[string]interface{}{}
	err = json.Unmarshal(body, &actualResponseObject)
	require.NoError(t, err)

	require.Equal(t, expectedResponseObject, actualResponseObject)
}

func Test_UpdateAWSResourceWithPost_NoChangesNoops(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	testOptions := setupTest(t)
	primaryIdentifiers := map[string]interface{}{
		"primaryIdentifier": []interface{}{
			"/properties/Name",
		},
	}
	serialized, err := json.Marshal(primaryIdentifiers)
	require.NoError(t, err)
	output := cloudformation.DescribeTypeOutput{
		TypeName: aws.String("AWS::Kinesis::Stream"),
		Schema:   aws.String(string(serialized)),
	}

	testOptions.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any()).Return(&output, nil)

	getResponseBody := map[string]interface{}{
		"Name":                 testAWSResourceName,
		"RetentionPeriodHours": 178,
		"ShardCount":           3,
	}
	getResponseBodyBytes, err := json.Marshal(getResponseBody)
	require.NoError(t, err)

	testOptions.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudcontrol.GetResourceOutput{
			ResourceDescription: &types.ResourceDescription{
				Properties: aws.String(string(getResponseBodyBytes)),
			},
		}, nil)

	requestBody := map[string]interface{}{
		"properties": map[string]interface{}{
			"Name":                 testAWSResourceName,
			"RetentionPeriodHours": 178,
			"ShardCount":           3,
		},
	}
	requestBodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	awsController, err := NewCreateOrUpdateAWSResourceWithPost(ctrl.Options{
		AWSCloudControlClient:   testOptions.AWSCloudControlClient,
		AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		DB:                      testOptions.StorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, testAWSResourceCollectionPath, bytes.NewBuffer(requestBodyBytes))
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

	expectedResponseObject := map[string]interface{}{
		"id":   testAWSSingleResourcePath,
		"name": testAWSResourceName,
		"type": testAWSResourceType,
		"properties": map[string]interface{}{
			"Name":                 testAWSResourceName,
			"RetentionPeriodHours": float64(178),
			"ShardCount":           float64(3),
			"provisioningState":    "Succeeded",
		},
	}

	actualResponseObject := map[string]interface{}{}
	err = json.Unmarshal(body, &actualResponseObject)
	require.NoError(t, err)

	require.Equal(t, expectedResponseObject, actualResponseObject)
}

func Test_CreateAWSResourceWithPost_MultiIdentifier(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	testOptions := setupTest(t)

	primaryIdentifiers := map[string]interface{}{
		"primaryIdentifier": []interface{}{
			"/properties/ClusterIdentifier",
			"/properties/Account",
		},
	}
	serialized, err := json.Marshal(primaryIdentifiers)
	require.NoError(t, err)
	output := cloudformation.DescribeTypeOutput{
		TypeName: aws.String("AWS::RedShift::EndpointAuthorization"),
		Schema:   aws.String(string(serialized)),
	}

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
			},
		}, nil)

	requestBody := map[string]interface{}{
		"properties": map[string]interface{}{
			"ClusterIdentifier": testPrimaryIdentifier1,
			"Account":           testPrimaryIdentifier2,
		},
	}
	requestBodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	awsController, err := NewCreateOrUpdateAWSResourceWithPost(ctrl.Options{
		AWSCloudControlClient:   testOptions.AWSCloudControlClient,
		AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		DB:                      testOptions.StorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, testMultiIdentifierResourcePath+"/:put", bytes.NewBuffer(requestBodyBytes))
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

	id, err := resources.Parse(testMultiIdentifierResourcePath)
	require.NoError(t, err)
	multiIdentifierResourceID := testPrimaryIdentifier1 + "|" + testPrimaryIdentifier2
	rID := computeResourceID(id, multiIdentifierResourceID)
	expectedResponseObject := map[string]interface{}{
		"id":   rID,
		"name": multiIdentifierResourceID,
		"type": testMultiIdentifierResourceType,
		"properties": map[string]interface{}{
			"ClusterIdentifier": "abc",
			"Account":           "xyz",
			"provisioningState": "Provisioning",
		},
	}

	actualResponseObject := map[string]interface{}{}
	err = json.Unmarshal(body, &actualResponseObject)
	require.NoError(t, err)

	require.Equal(t, expectedResponseObject, actualResponseObject)
}

func Test_UpdateAWSResourceWithPost_MultiIdentifier(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	testOptions := setupTest(t)
	primaryIdentifiers := map[string]interface{}{
		"primaryIdentifier": []interface{}{
			"/properties/ClusterIdentifier",
			"/properties/Account",
		},
	}
	serialized, err := json.Marshal(primaryIdentifiers)
	require.NoError(t, err)
	output := cloudformation.DescribeTypeOutput{
		TypeName: aws.String("AWS::RedShift::EndpointAuthorization"),
		Schema:   aws.String(string(serialized)),
	}

	testOptions.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any()).Return(&output, nil)

	getResponseBody := map[string]interface{}{
		"ClusterIdentifier": "abc",
		"Account":           "xyz",
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
			},
		}, nil)

	requestBody := map[string]interface{}{
		"properties": map[string]interface{}{
			"ClusterIdentifier": "abc",
			"Account":           "xyz",
			"EndpointCount":     2,
		},
	}
	requestBodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	awsController, err := NewCreateOrUpdateAWSResourceWithPost(ctrl.Options{
		AWSCloudControlClient:   testOptions.AWSCloudControlClient,
		AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		DB:                      testOptions.StorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, testMultiIdentifierResourcePath, bytes.NewBuffer(requestBodyBytes))
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

	id, err := resources.Parse(testMultiIdentifierResourcePath)
	require.NoError(t, err)
	multiIdentifierResourceID := testPrimaryIdentifier1 + "|" + testPrimaryIdentifier2
	rID := computeResourceID(id, multiIdentifierResourceID)
	expectedResponseObject := map[string]interface{}{
		"id":   rID,
		"name": multiIdentifierResourceID,
		"type": testMultiIdentifierResourceType,
		"properties": map[string]interface{}{
			"ClusterIdentifier": "abc",
			"Account":           "xyz",
			"EndpointCount":     float64(2),
			"provisioningState": "Provisioning",
		},
	}

	actualResponseObject := map[string]interface{}{}
	err = json.Unmarshal(body, &actualResponseObject)
	require.NoError(t, err)

	require.Equal(t, expectedResponseObject, actualResponseObject)
}

func Test_UpdateAWSResourceWithPost_WithSpecialProperties(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	resourceTypeAWS := "AWS::MemoryDB::Cluster"
	resourceId := "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/AWS.MemoryDB/Cluster/mycluster"
	resourceType := "AWS.MemoryDB/Cluster"
	resourceName := "mycluster"

	testOptions := setupTest(t)
	typeSchema := map[string]interface{}{
		"primaryIdentifier": []interface{}{
			"/properties/ClusterName",
		},
		"readOnlyProperties": []interface{}{
			"/properties/ClusterEndpoint/Address",
			"/properties/ClusterEndpoint/Port",
			"/properties/ARN",
		},
		"createOnlyProperties": []interface{}{
			"/properties/ClusterName",
			"/properties/Port",
		},
	}
	serialized, err := json.Marshal(typeSchema)
	require.NoError(t, err)
	output := cloudformation.DescribeTypeOutput{
		TypeName: aws.String(resourceTypeAWS),
		Schema:   aws.String(string(serialized)),
	}

	testOptions.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any()).Return(&output, nil)

	getResponseBody := map[string]interface{}{
		"ClusterName": resourceName,
		"ClusterEndpoint": map[string]interface{}{
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

	testOptions.AWSCloudControlClient.EXPECT().UpdateResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudcontrol.UpdateResourceOutput{
			ProgressEvent: &types.ProgressEvent{
				OperationStatus: types.OperationStatusSuccess,
				RequestToken:    aws.String(testAWSRequestToken),
			},
		}, nil)

	requestBody := map[string]interface{}{
		"properties": map[string]interface{}{
			"ClusterName":         resourceName,
			"Port":                6379,
			"NumReplicasPerShard": 0,
		},
	}
	requestBodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	awsController, err := NewCreateOrUpdateAWSResourceWithPost(ctrl.Options{
		AWSCloudControlClient:   testOptions.AWSCloudControlClient,
		AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		DB:                      testOptions.StorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/AWS.MemoryDB/Cluster", bytes.NewBuffer(requestBodyBytes))
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
		"id":   resourceId,
		"name": resourceName,
		"type": resourceType,
		"properties": map[string]interface{}{
			"ClusterName": resourceName,
			"ClusterEndpoint": map[string]interface{}{
				"Address": "test",
				"Port":    float64(6379),
			},
			"Port":                float64(6379),
			"ARN":                 "arn:aws:memorydb:us-west-2:123456789012:cluster:mycluster",
			"NumReplicasPerShard": float64(0),
			"provisioningState":   "Provisioning",
		},
	}

	actualResponseObject := map[string]interface{}{}
	err = json.Unmarshal(body, &actualResponseObject)
	require.NoError(t, err)

	require.Equal(t, expectedResponseObject, actualResponseObject)
}

func Test_UpdateAWSResourceWithPost_WithSpecialProperties_NoChangesNoops(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	resourceTypeAWS := "AWS::MemoryDB::Cluster"
	resourceId := "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/AWS.MemoryDB/Cluster/mycluster"
	resourceType := "AWS.MemoryDB/Cluster"
	resourceName := "mycluster"

	testOptions := setupTest(t)
	typeSchema := map[string]interface{}{
		"primaryIdentifier": []interface{}{
			"/properties/ClusterName",
		},
		"readOnlyProperties": []interface{}{
			"/properties/ClusterEndpoint/Address",
			"/properties/ClusterEndpoint/Port",
			"/properties/ARN",
		},
		"createOnlyProperties": []interface{}{
			"/properties/ClusterName",
			"/properties/Port",
		},
	}
	serialized, err := json.Marshal(typeSchema)
	require.NoError(t, err)
	output := cloudformation.DescribeTypeOutput{
		TypeName: aws.String(resourceTypeAWS),
		Schema:   aws.String(string(serialized)),
	}

	testOptions.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any()).Return(&output, nil)

	getResponseBody := map[string]interface{}{
		"ClusterName": resourceName,
		"ClusterEndpoint": map[string]interface{}{
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

	requestBody := map[string]interface{}{
		"properties": map[string]interface{}{
			"ClusterName":         resourceName,
			"Port":                6379,
			"NumReplicasPerShard": 1,
		},
	}
	requestBodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	awsController, err := NewCreateOrUpdateAWSResourceWithPost(ctrl.Options{
		AWSCloudControlClient:   testOptions.AWSCloudControlClient,
		AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		DB:                      testOptions.StorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/AWS.MemoryDB/Cluster", bytes.NewBuffer(requestBodyBytes))
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

	expectedResponseObject := map[string]interface{}{
		"id":   resourceId,
		"name": resourceName,
		"type": resourceType,
		"properties": map[string]interface{}{
			"ClusterName": resourceName,
			"ClusterEndpoint": map[string]interface{}{
				"Address": "test",
				"Port":    float64(6379),
			},
			"Port":                float64(6379),
			"ARN":                 "arn:aws:memorydb:us-west-2:123456789012:cluster:mycluster",
			"NumReplicasPerShard": float64(1),
			"provisioningState":   "Succeeded",
		},
	}

	actualResponseObject := map[string]interface{}{}
	err = json.Unmarshal(body, &actualResponseObject)
	require.NoError(t, err)

	require.Equal(t, expectedResponseObject, actualResponseObject)
}
