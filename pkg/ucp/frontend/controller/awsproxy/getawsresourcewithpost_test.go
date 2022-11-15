// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package awsproxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/golang/mock/gomock"
	"gotest.tools/assert"

	armrpc_v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
)

func Test_GetAWSResourceWithPost(t *testing.T) {
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
		Schema:   to.Ptr(string(serialized)),
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
				Identifier: aws.String(testAWSResourceName),
				Properties: aws.String(string(getResponseBodyBytes)),
			},
		}, nil)

	awsController, err := NewGetAWSResourceWithPost(ctrl.Options{
		AWSCloudControlClient:   testOptions.AWSCloudControlClient,
		AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		DB:                      testOptions.StorageClient,
	})
	require.NoError(t, err)

	requestBody := map[string]interface{}{
		"properties": map[string]interface{}{
			"Name": testAWSResourceName,
		},
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, testAWSResourceCollectionPath+"/:get", bytes.NewBuffer(body))

	require.NoError(t, err)
	actualResponse, err := awsController.Run(ctx, nil, request)

	expectedResponse := armrpc_rest.NewOKResponse(map[string]interface{}{
		"id":   testAWSSingleResourcePath,
		"type": testAWSResourceType,
		"name": to.Ptr(testAWSResourceName),
		"properties": map[string]interface{}{
			"Name":                 testAWSResourceName,
			"RetentionPeriodHours": float64(178),
			"ShardCount":           float64(3),
		},
	})

	require.NoError(t, err)
	assert.DeepEqual(t, expectedResponse, actualResponse)
}

func Test_GetAWSResourceWithPost_NotFound(t *testing.T) {
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
		Schema:   to.Ptr(string(serialized)),
	}

	testOptions.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any()).Return(&output, nil)

	testOptions.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil, &types.ResourceNotFoundException{
			Message: aws.String("Resource not found"),
		})

	awsController, err := NewGetAWSResourceWithPost(ctrl.Options{
		AWSCloudControlClient:   testOptions.AWSCloudControlClient,
		AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		DB:                      testOptions.StorageClient,
	})
	require.NoError(t, err)

	requestBody := map[string]interface{}{
		"properties": map[string]interface{}{
			"Name": testAWSResourceName,
		},
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, testAWSResourceCollectionPath+"/:get", bytes.NewBuffer(body))

	require.NoError(t, err)
	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	expectedResponse := armrpc_rest.NewNotFoundMessageResponse(fmt.Sprintf("Resource %s with primary identifiers %s not found", testAWSResourceCollectionPath, testAWSResourceName))
	require.Equal(t, expectedResponse, actualResponse)
}

func Test_GetAWSResourceWithPost_UnknownError(t *testing.T) {
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
		Schema:   to.Ptr(string(serialized)),
	}

	testOptions.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any()).Return(&output, nil)

	testOptions.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any()).Return(nil, errors.New("something bad happened"))

	awsController, err := NewGetAWSResourceWithPost(ctrl.Options{
		AWSCloudControlClient:   testOptions.AWSCloudControlClient,
		AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		DB:                      testOptions.StorageClient,
	})
	require.NoError(t, err)

	requestBody := map[string]interface{}{
		"properties": map[string]interface{}{
			"Name": testAWSResourceName,
		},
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, testAWSResourceCollectionPath+"/:get", bytes.NewBuffer(body))
	require.NoError(t, err)

	actualResponse, err := awsController.Run(ctx, nil, request)
	require.Error(t, err)

	require.Nil(t, actualResponse)
	require.Equal(t, "something bad happened", err.Error())
}

func Test_GetAWSResourceWithPost_SmithyError(t *testing.T) {
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
		Schema:   to.Ptr(string(serialized)),
	}

	testOptions.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any()).Return(&output, nil)

	testOptions.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any()).Return(nil, &smithy.OperationError{
		Err: &smithyhttp.ResponseError{
			Err: &smithy.GenericAPIError{
				Code:    "NotFound",
				Message: "Resource not found",
			},
		},
	})

	awsController, err := NewGetAWSResourceWithPost(ctrl.Options{
		AWSCloudControlClient:   testOptions.AWSCloudControlClient,
		AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		DB:                      testOptions.StorageClient,
	})
	require.NoError(t, err)

	requestBody := map[string]interface{}{
		"properties": map[string]interface{}{
			"Name": testAWSResourceName,
		},
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, testAWSResourceCollectionPath+"/:get", bytes.NewBuffer(body))
	require.NoError(t, err)

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
