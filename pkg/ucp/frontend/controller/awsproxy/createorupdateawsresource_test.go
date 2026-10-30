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
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"uuid"

	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	armrpc_v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_CreateAWSResourceInvalidRegion(t *testing.T) {
	testResource := CreateKinesisStreamTestResourceWithInvalidRegion(uuid.New().String())
	testOptions := setupTest(t)

	requestBody := map[string]any{
		"properties": map[string]any{
			"RetentionPeriodHours": 178,
			"ShardCount":           3,
		},
	}
	requestBodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	awsClients := ucp_aws.Clients{
		CloudControl:   testOptions.AWSCloudControlClient,
		CloudFormation: testOptions.AWSCloudFormationClient,
	}
	awsController, err := NewCreateOrUpdateAWSResource(testOptions.ControllerOptions(), awsClients)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPut, testResource.SingleResourcePath, bytes.NewBuffer(requestBodyBytes))
	request.Host = testHost
	request.URL.Host = testHost
	request.URL.Scheme = testScheme

	require.NoError(t, err)

	ctx := rpctest.NewARMRequestContext(request)
	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	err = actualResponse.Apply(ctx, w, request)
	require.NoError(t, err)

	res := w.Result()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)

	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	expectedResponseObject := map[string]any{
		"error": map[string]any{
			"code":    "BadRequest",
			"message": "failed to read region from request path: 'regions' not found",
		},
	}

	actualResponseObject := map[string]any{}
	err = json.Unmarshal(body, &actualResponseObject)
	require.NoError(t, err)

	require.Equal(t, expectedResponseObject, actualResponseObject)
}

func Test_CreateAWSResource_DescribeTypeError(t *testing.T) {
	testResource := CreateKinesisStreamTestResource(uuid.New().String())

	testOptions := setupTest(t)
	// DescribeType runs before UCP reads the resource, so its error stops the request before any Cloud Control call.
	testOptions.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &smithy.OperationError{
		Err: &smithyhttp.ResponseError{
			Err: &smithy.GenericAPIError{
				Code:    "AccessDeniedException",
				Message: "not authorized to perform cloudformation:DescribeType",
				Fault:   smithy.FaultClient,
			},
		},
	})

	awsController, err := NewCreateOrUpdateAWSResource(testOptions.ControllerOptions(), ucp_aws.Clients{
		CloudControl:   testOptions.AWSCloudControlClient,
		CloudFormation: testOptions.AWSCloudFormationClient,
	})
	require.NoError(t, err)

	requestBody, err := json.Marshal(map[string]any{"properties": map[string]any{"ShardCount": 3}})
	require.NoError(t, err)
	request, err := http.NewRequest(http.MethodPut, testResource.SingleResourcePath, bytes.NewBuffer(requestBody))
	require.NoError(t, err)
	withOwner(request)

	ctx := rpctest.NewARMRequestContext(request)
	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	expectedResponse := armrpc_rest.NewBadRequestARMResponse(armrpc_v1.ErrorResponse{
		Error: &armrpc_v1.ErrorDetails{
			Code:    "AccessDeniedException",
			Message: "not authorized to perform cloudformation:DescribeType",
		},
	})
	require.Equal(t, expectedResponse, actualResponse)
}
