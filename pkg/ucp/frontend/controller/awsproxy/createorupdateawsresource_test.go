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

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/golang/mock/gomock"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"
)

func Test_CreateAWSResource(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	testOptions := setupTest(t)
	testOptions.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil, &types.ResourceNotFoundException{
			Message: aws.String("Resource not found"),
		})

	testOptions.AWSCloudControlClient.EXPECT().CreateResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudcontrol.CreateResourceOutput{
			ProgressEvent: &types.ProgressEvent{
				OperationStatus: types.OperationStatusSuccess,
				RequestToken:    to.StringPtr(testAWSRequestToken),
			},
		}, nil)

	requestBody := map[string]interface{}{
		"properties": map[string]interface{}{
			"RetentionPeriodHours": 178,
			"ShardCount":           3,
		},
	}
	requestBodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	awsController, err := NewCreateOrUpdateAWSResource(ctrl.Options{
		AWSCloudControlClient: testOptions.AWSCloudControlClient,
		DB:                    testOptions.StorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPut, testAWSSingleResourcePath, bytes.NewBuffer(requestBodyBytes))
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

func Test_UpdateAWSResource(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	resourceTypeAWS := "AWS::MemoryDB::Cluster"
	resourceId := "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/AWS.MemoryDB/Cluster/mycluster"
	resourceType := "AWS.MemoryDB/Cluster"
	resourceName := "mycluster"

	typeSchema := map[string]interface{}{
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

	getResponseBody := map[string]interface{}{
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

	requestBody := map[string]interface{}{
		"properties": map[string]interface{}{
			"NumReplicasPerShard": 0,
		},
	}
	requestBodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	awsController, err := NewCreateOrUpdateAWSResource(ctrl.Options{
		AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		AWSCloudControlClient:   testOptions.AWSCloudControlClient,
		DB:                      testOptions.StorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPut, resourceId, bytes.NewBuffer(requestBodyBytes))
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

func Test_UpdateNoChangesDoesNotCallUpdate(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	resourceType := "AWS::Kinesis::Stream"
	typeSchema := map[string]interface{}{
		"readOnlyProperties": []interface{}{
			"/properties/Arn",
		},
		"createOnlyProperties": []interface{}{
			"/properties/Name",
		},
	}
	serialized, err := json.Marshal(typeSchema)
	require.NoError(t, err)
	output := cloudformation.DescribeTypeOutput{
		TypeName: aws.String(resourceType),
		Schema:   aws.String(string(serialized)),
	}

	getResponseBody := map[string]interface{}{
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
				Properties: to.StringPtr(string(getResponseBodyBytes)),
			},
		}, nil)

	requestBody := map[string]interface{}{
		"properties": map[string]interface{}{
			"RetentionPeriodHours": 178,
			"ShardCount":           3,
		},
	}
	requestBodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	awsController, err := NewCreateOrUpdateAWSResource(ctrl.Options{
		AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		AWSCloudControlClient:   testOptions.AWSCloudControlClient,
		DB:                      testOptions.StorageClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPut, testAWSSingleResourcePath, bytes.NewBuffer(requestBodyBytes))
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

func Test_GeneratePatch(t *testing.T) {
	testCases := []struct {
		name          string
		currentState  map[string]interface{}
		desiredState  map[string]interface{}
		schema        map[string]interface{}
		expectedPatch jsondiff.Patch
	}{
		{
			"No updates creates empty patch",
			map[string]interface{}{
				"A": "B",
				"C": map[string]interface{}{
					"D": map[string]interface{}{
						"E": "F",
					},
					"G": map[string]interface{}{
						"I": "J",
					},
					"K": "L",
				},
			},
			map[string]interface{}{
				"A": "B",
				"C": map[string]interface{}{
					"G": map[string]interface{}{
						"I": "J",
					},
				},
			},
			map[string]interface{}{
				"properties": map[string]interface{}{
					"A": map[string]interface{}{},
					"C": map[string]interface{}{},
				},
				"readOnlyProperties": []interface{}{
					"/properties/C/D/E",
				},
				"createOnlyProperties": []interface{}{
					"/properties/C/K",
				},
			},
			nil,
		},
		{
			"Update creates patch",
			map[string]interface{}{
				"A": "B",
				"C": map[string]interface{}{
					"D": map[string]interface{}{
						"E": "F",
					},
					"G": map[string]interface{}{
						"I": "J",
					},
					"K": "L",
				},
			},
			map[string]interface{}{
				"A": "Test",
				"C": map[string]interface{}{
					"G": map[string]interface{}{
						"I": "Test2",
					},
				},
			},
			map[string]interface{}{
				"properties": map[string]interface{}{
					"A": map[string]interface{}{},
					"C": map[string]interface{}{},
				},
				"readOnlyProperties": []interface{}{
					"/properties/C/D/E",
				},
				"createOnlyProperties": []interface{}{
					"/properties/C/K",
				},
			},
			jsondiff.Patch{
				{
					Type:     "replace",
					Path:     "/A",
					OldValue: "B",
					Value:    "Test",
				},
				{
					Type:     "replace",
					Path:     "/C/G/I",
					OldValue: "J",
					Value:    "Test2",
				},
			},
		},
		{
			"Specify create-only properties",
			map[string]interface{}{
				"A": map[string]interface{}{
					"B": map[string]interface{}{
						"C": "D",
						"E": "F",
					},
				},
			},
			map[string]interface{}{
				"A": map[string]interface{}{
					"B": map[string]interface{}{
						"C": "D",
						"E": "Test",
					},
				},
			},
			map[string]interface{}{
				"properties": map[string]interface{}{
					"A": map[string]interface{}{},
				},
				"createOnlyProperties": []interface{}{
					"/properties/A/B/C",
				},
			},
			jsondiff.Patch{
				{
					Type:     "replace",
					Path:     "/A/B/E",
					OldValue: "F",
					Value:    "Test",
				},
			},
		},
		{
			"Remove object",
			map[string]interface{}{
				"A": map[string]interface{}{
					"B": map[string]interface{}{
						"C": "D",
						"E": "F",
					},
				},
				"G": "H",
			},
			map[string]interface{}{
				"G": "H",
			},
			map[string]interface{}{
				"properties": map[string]interface{}{
					"A": map[string]interface{}{},
					"G": map[string]interface{}{},
				},
			},
			jsondiff.Patch{
				{
					Type: "remove",
					Path: "/A",
					OldValue: map[string]interface{}{
						"B": map[string]interface{}{
							"C": "D",
							"E": "F",
						},
					},
					Value: nil,
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			desiredStateBytes, err := json.Marshal(testCase.desiredState)
			require.NoError(t, err)

			currentStateBytes, err := json.Marshal(testCase.currentState)
			require.NoError(t, err)

			schemaBytes, err := json.Marshal(testCase.schema)
			require.NoError(t, err)

			patch, err := generatePatch(currentStateBytes, desiredStateBytes, schemaBytes)
			require.NoError(t, err)

			require.Equal(t, testCase.expectedPatch, patch)
		})

	}
}
