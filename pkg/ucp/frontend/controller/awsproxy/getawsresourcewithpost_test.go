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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"

	armrpc_v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/armrpc/rpctest"
	ucp_aws "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

func Test_GetAWSResourceWithPost(t *testing.T) {
	testResource := CreateKinesisStreamTestResource(uuid.NewString())

	output := cloudformation.DescribeTypeOutput{
		TypeName: aws.String(testResource.AWSResourceType),
		Schema:   aws.String(testResource.Schema),
	}

	testOptions := setupTest(t)
	testOptions.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).Return(&output, nil)

	getResponseBody := map[string]any{
		"Name":                 testResource.ResourceName,
		"RetentionPeriodHours": 178,
		"ShardCount":           3,
	}
	getResponseBodyBytes, err := json.Marshal(getResponseBody)
	require.NoError(t, err)

	testOptions.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudcontrol.GetResourceOutput{
			ResourceDescription: &types.ResourceDescription{
				Identifier: aws.String(testResource.ResourceName),
				Properties: aws.String(string(getResponseBodyBytes)),
			},
		}, nil)

	awsClients := ucp_aws.Clients{
		CloudControl:   testOptions.AWSCloudControlClient,
		CloudFormation: testOptions.AWSCloudFormationClient,
	}
	awsController, err := NewGetAWSResourceWithPost(armrpc_controller.Options{StorageClient: testOptions.StorageClient}, awsClients)
	require.NoError(t, err)

	requestBody := map[string]any{
		"properties": map[string]any{
			"Name": testResource.ResourceName,
		},
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, testResource.CollectionPath+"/:get", bytes.NewBuffer(body))
	require.NoError(t, err)

	ctx := rpctest.ARMTestContextFromRequest(request)
	actualResponse, err := awsController.Run(ctx, nil, request)

	expectedResponse := armrpc_rest.NewOKResponse(map[string]any{
		"id":   testResource.SingleResourcePath,
		"type": testResource.ResourceType,
		"name": aws.String(testResource.ResourceName),
		"properties": map[string]any{
			"Name":                 testResource.ResourceName,
			"RetentionPeriodHours": float64(178),
			"ShardCount":           float64(3),
		},
	})

	require.NoError(t, err)
	require.Equal(t, expectedResponse, actualResponse)
}

func Test_GetAWSResourceWithPost_NotFound(t *testing.T) {
	testResource := CreateKinesisStreamTestResource(uuid.NewString())

	output := cloudformation.DescribeTypeOutput{
		TypeName: aws.String(testResource.AWSResourceType),
		Schema:   aws.String(testResource.Schema),
	}

	testOptions := setupTest(t)
	testOptions.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).Return(&output, nil)

	testOptions.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil, &types.ResourceNotFoundException{
			Message: aws.String("Resource not found"),
		})

	awsClients := ucp_aws.Clients{
		CloudControl:   testOptions.AWSCloudControlClient,
		CloudFormation: testOptions.AWSCloudFormationClient,
	}
	awsController, err := NewGetAWSResourceWithPost(armrpc_controller.Options{StorageClient: testOptions.StorageClient}, awsClients)
	require.NoError(t, err)

	requestBody := map[string]any{
		"properties": map[string]any{
			"Name": testResource.ResourceName,
		},
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, testResource.CollectionPath+"/:get", bytes.NewBuffer(body))
	require.NoError(t, err)

	ctx := rpctest.ARMTestContextFromRequest(request)
	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	expectedResponse := armrpc_rest.NewNotFoundMessageResponse(fmt.Sprintf("Resource %s with primary identifiers %s not found", testResource.CollectionPath, testResource.ResourceName))
	require.Equal(t, expectedResponse, actualResponse)
}

func Test_GetAWSResourceWithPost_UnknownError(t *testing.T) {
	testResource := CreateKinesisStreamTestResource(uuid.NewString())

	output := cloudformation.DescribeTypeOutput{
		TypeName: aws.String(testResource.AWSResourceType),
		Schema:   aws.String(testResource.Schema),
	}

	testOptions := setupTest(t)
	testOptions.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).Return(&output, nil)

	testOptions.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("something bad happened"))

	awsClients := ucp_aws.Clients{
		CloudControl:   testOptions.AWSCloudControlClient,
		CloudFormation: testOptions.AWSCloudFormationClient,
	}
	awsController, err := NewGetAWSResourceWithPost(armrpc_controller.Options{StorageClient: testOptions.StorageClient}, awsClients)
	require.NoError(t, err)

	requestBody := map[string]any{
		"properties": map[string]any{
			"Name": testResource.ResourceName,
		},
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, testResource.CollectionPath+"/:get", bytes.NewBuffer(body))
	require.NoError(t, err)

	ctx := rpctest.ARMTestContextFromRequest(request)
	actualResponse, err := awsController.Run(ctx, nil, request)
	require.Error(t, err)

	require.Nil(t, actualResponse)
	require.Equal(t, "something bad happened", err.Error())
}

func Test_GetAWSResourceWithPost_SmithyError(t *testing.T) {
	testResource := CreateKinesisStreamTestResource(uuid.NewString())

	output := cloudformation.DescribeTypeOutput{
		TypeName: aws.String(testResource.AWSResourceType),
		Schema:   aws.String(testResource.Schema),
	}

	testOptions := setupTest(t)
	testOptions.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).Return(&output, nil)

	testOptions.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &smithy.OperationError{
		Err: &smithyhttp.ResponseError{
			Err: &smithy.GenericAPIError{
				Code:    "NotFound",
				Message: "Resource not found",
			},
		},
	})

	awsClients := ucp_aws.Clients{
		CloudControl:   testOptions.AWSCloudControlClient,
		CloudFormation: testOptions.AWSCloudFormationClient,
	}
	awsController, err := NewGetAWSResourceWithPost(armrpc_controller.Options{StorageClient: testOptions.StorageClient}, awsClients)
	require.NoError(t, err)

	requestBody := map[string]any{
		"properties": map[string]any{
			"Name": testResource.ResourceName,
		},
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, testResource.CollectionPath+"/:get", bytes.NewBuffer(body))
	require.NoError(t, err)

	ctx := rpctest.ARMTestContextFromRequest(request)
	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	expectedResponse := armrpc_rest.NewInternalServerErrorARMResponse(armrpc_v1.ErrorResponse{
		Error: armrpc_v1.ErrorDetails{
			Code:    "NotFound",
			Message: "Resource not found",
		},
	})

	require.Equal(t, expectedResponse, actualResponse)
}

func Test_GetAWSResourceWithPost_MultiIdentifier(t *testing.T) {
	testResource := CreateRedshiftEndpointAuthorizationTestResource(uuid.NewString())
	clusterIdentifierValue := "abc"
	accountValue := "xyz"
	requestBody := map[string]any{
		"properties": map[string]any{
			"ClusterIdentifier": clusterIdentifierValue,
			"Account":           accountValue,
		},
	}
	requestBodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, testResource.CollectionPath+"/:get", bytes.NewBuffer(requestBodyBytes))
	require.NoError(t, err)
	ctx := rpctest.ARMTestContextFromRequest(request)

	output := cloudformation.DescribeTypeOutput{
		TypeName: aws.String(testResource.AWSResourceType),
		Schema:   aws.String(testResource.Schema),
	}

	testOptions := setupTest(t)
	testOptions.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).Return(&output, nil)

	getResponseBody := map[string]any{
		"ClusterIdentifier": clusterIdentifierValue,
		"Account":           accountValue,
	}
	getResponseBodyBytes, err := json.Marshal(getResponseBody)
	require.NoError(t, err)

	testOptions.AWSCloudControlClient.EXPECT().GetResource(ctx, &cloudcontrol.GetResourceInput{
		TypeName:   aws.String(testResource.AWSResourceType),
		Identifier: aws.String("abc|xyz"),
	}, gomock.Any()).Return(
		&cloudcontrol.GetResourceOutput{
			ResourceDescription: &types.ResourceDescription{
				Identifier: aws.String(testResource.ResourceName),
				Properties: aws.String(string(getResponseBodyBytes)),
			},
		}, nil)

	awsClients := ucp_aws.Clients{
		CloudControl:   testOptions.AWSCloudControlClient,
		CloudFormation: testOptions.AWSCloudFormationClient,
	}

	awsController, err := NewGetAWSResourceWithPost(armrpc_controller.Options{StorageClient: testOptions.StorageClient}, awsClients)
	require.NoError(t, err)

	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	err = actualResponse.Apply(ctx, w, request)
	require.NoError(t, err)

	res := w.Result()
	require.Equal(t, http.StatusOK, res.StatusCode)
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	id, err := resources.Parse(testResource.CollectionPath)
	require.NoError(t, err)
	multiIdentifierResourceID := clusterIdentifierValue + "|" + accountValue
	rID := computeResourceID(id, multiIdentifierResourceID)
	expectedResponseObject := map[string]any{
		"id":   rID,
		"name": testResource.ResourceName,
		"type": testResource.ResourceType,
		"properties": map[string]any{
			"ClusterIdentifier": "abc",
			"Account":           "xyz",
		},
	}

	actualResponseObject := map[string]any{}
	err = json.Unmarshal(body, &actualResponseObject)
	require.NoError(t, err)

	require.Equal(t, expectedResponseObject, actualResponseObject)
}
