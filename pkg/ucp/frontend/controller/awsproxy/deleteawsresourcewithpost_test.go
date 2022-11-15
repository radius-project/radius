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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/golang/mock/gomock"

	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
)

func Test_DeleteAWSResourceWithPost(t *testing.T) {
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

	testOptions.AWSCloudControlClient.EXPECT().DeleteResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudcontrol.DeleteResourceOutput{
			ProgressEvent: &types.ProgressEvent{
				OperationStatus: types.OperationStatusSuccess,
				RequestToken:    aws.String(testAWSRequestToken),
			},
		}, nil)

	awsController, err := NewDeleteAWSResourceWithPost(ctrl.Options{
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

	request, err := http.NewRequest(http.MethodPost, testAWSResourceCollectionPath+"/:delete", bytes.NewBuffer(body))
	require.NoError(t, err)

	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	err = actualResponse.Apply(ctx, w, request)
	require.NoError(t, err)

	res := w.Result()
	require.Equal(t, http.StatusAccepted, res.StatusCode)
	body, err = io.ReadAll(res.Body)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, []byte("{}"), body)
}

func Test_DeleteAWSResourceWithPost_ResourceDoesNotExist(t *testing.T) {
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

	awsController, err := NewDeleteAWSResourceWithPost(ctrl.Options{
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

	request, err := http.NewRequest(http.MethodPost, testAWSResourceCollectionPath+"/:delete", bytes.NewBuffer(body))
	require.NoError(t, err)

	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", testAWSSingleResourcePath, nil)
	w := httptest.NewRecorder()
	err = actualResponse.Apply(ctx, w, req)
	require.NoError(t, err)

	res := w.Result()
	require.Equal(t, http.StatusNoContent, res.StatusCode)
	body, err = io.ReadAll(res.Body)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, []byte(""), body)
}
