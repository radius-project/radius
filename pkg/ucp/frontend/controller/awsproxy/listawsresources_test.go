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
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"

	armrpc_v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/test/testutil"
	"github.com/stretchr/testify/require"
)

func Test_ListAWSResources(t *testing.T) {

	firstTestResource := CreateKinesisStreamTestResource(uuid.NewString())
	secondTestResource := CreateKinesisStreamTestResource(uuid.NewString())

	firstTestResourceResponseBody := map[string]any{
		"RetentionPeriodHours": 178,
		"ShardCount":           3,
	}
	firstTestResourceResponseBodyBytes, err := json.Marshal(firstTestResourceResponseBody)
	require.NoError(t, err)

	secondTestResourceResponseBody := map[string]any{
		"RetentionPeriodHours": 180,
		"ShardCount":           2,
	}
	secondTestResourceResponseBodyBytes, err := json.Marshal(secondTestResourceResponseBody)
	require.NoError(t, err)

	testOptions := setupTest(t)
	testOptions.AWSCloudControlClient.EXPECT().ListResources(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudcontrol.ListResourcesOutput{
			ResourceDescriptions: []types.ResourceDescription{
				{
					Identifier: aws.String(firstTestResource.ResourceName),
					Properties: aws.String(string(firstTestResourceResponseBodyBytes)),
				},
				{
					Identifier: aws.String(secondTestResource.ResourceName),
					Properties: aws.String(string(secondTestResourceResponseBodyBytes)),
				},
			},
		}, nil)

	awsController, err := NewListAWSResources(ctrl.Options{
		AWSOptions: ctrl.AWSOptions{
			AWSCloudControlClient:   testOptions.AWSCloudControlClient,
			AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		},
		Options: armrpc_controller.Options{
			StorageClient: testOptions.StorageClient,
		},
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, firstTestResource.CollectionPath, nil)
	require.NoError(t, err)

	ctx := testutil.ARMTestContextFromRequest(request)
	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	expectedResponse := armrpc_rest.NewOKResponse(map[string]any{
		"value": []any{
			map[string]any{
				"id":   firstTestResource.SingleResourcePath,
				"name": aws.String(firstTestResource.ResourceName),
				"type": firstTestResource.ResourceType,
				"properties": map[string]any{
					"RetentionPeriodHours": float64(178),
					"ShardCount":           float64(3),
				},
			},
			map[string]any{
				"id":   secondTestResource.SingleResourcePath,
				"name": aws.String(secondTestResource.ResourceName),
				"type": secondTestResource.ResourceType,
				"properties": map[string]any{
					"RetentionPeriodHours": float64(180),
					"ShardCount":           float64(2),
				},
			},
		},
	})

	require.Equal(t, expectedResponse, actualResponse)
}

func Test_ListAWSResourcesEmpty(t *testing.T) {
	testResource := CreateKinesisStreamTestResource(uuid.NewString())

	testOptions := setupTest(t)
	testOptions.AWSCloudControlClient.EXPECT().ListResources(gomock.Any(), gomock.Any(), gomock.Any()).Return(&cloudcontrol.ListResourcesOutput{}, nil)

	awsController, err := NewListAWSResources(ctrl.Options{
		AWSOptions: ctrl.AWSOptions{
			AWSCloudControlClient:   testOptions.AWSCloudControlClient,
			AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		},
		Options: armrpc_controller.Options{
			StorageClient: testOptions.StorageClient,
		},
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, testResource.CollectionPath, nil)
	require.NoError(t, err)

	ctx := testutil.ARMTestContextFromRequest(request)
	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	expectedResponse := armrpc_rest.NewOKResponse(map[string]any{
		"value": []any{},
	})

	require.Equal(t, expectedResponse, actualResponse)
}

func Test_ListAWSResource_UnknownError(t *testing.T) {
	testResource := CreateKinesisStreamTestResource(uuid.NewString())

	testOptions := setupTest(t)
	testOptions.AWSCloudControlClient.EXPECT().ListResources(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("something bad happened"))

	awsController, err := NewListAWSResources(ctrl.Options{
		AWSOptions: ctrl.AWSOptions{
			AWSCloudControlClient:   testOptions.AWSCloudControlClient,
			AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		},
		Options: armrpc_controller.Options{
			StorageClient: testOptions.StorageClient,
		},
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, testResource.CollectionPath, nil)
	require.NoError(t, err)

	ctx := testutil.ARMTestContextFromRequest(request)
	actualResponse, err := awsController.Run(ctx, nil, request)
	require.Error(t, err)

	require.Nil(t, actualResponse)
	require.Equal(t, "something bad happened", err.Error())
}

func Test_ListAWSResource_SmithyError(t *testing.T) {
	testResource := CreateKinesisStreamTestResource(uuid.NewString())

	testOptions := setupTest(t)
	testOptions.AWSCloudControlClient.EXPECT().ListResources(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &smithy.OperationError{
		Err: &smithyhttp.ResponseError{
			Err: &smithy.GenericAPIError{
				Code:    "NotFound",
				Message: "Resource not found",
			},
		},
	})

	awsController, err := NewListAWSResources(ctrl.Options{
		AWSOptions: ctrl.AWSOptions{
			AWSCloudControlClient:   testOptions.AWSCloudControlClient,
			AWSCloudFormationClient: testOptions.AWSCloudFormationClient,
		},
		Options: armrpc_controller.Options{
			StorageClient: testOptions.StorageClient,
		},
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, testResource.CollectionPath, nil)
	require.NoError(t, err)

	ctx := testutil.ARMTestContextFromRequest(request)
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
