// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package aws

// Tests that test with Mock RP functionality and UCP Server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/mux"
	"github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/frontend/api"
	"github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type Client struct {
	httpClient *http.Client
	baseURL    string
}

func NewClient(httpClient *http.Client, baseURL string) Client {
	return Client{
		httpClient: httpClient,
		baseURL:    baseURL,
	}
}

const (
	testProxyRequestAWSPath      = "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/Stream/stream-1"
	testAWSResourceName          = "stream-1"
	testAWSResourceType          = "AWS.Kinesis/Stream"
	testProxyRequestAWSAsyncPath = "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/locations/global"
	testAWSPlaneID               = "/planes/aws/aws"
	testAWSRequestToken          = "79B9F0DA-4882-4DC8-A367-6FD3BC122DED" // Random UUID
	basePath                     = "/apis/api.ucp.dev/v1alpha3"
)

func Test_CreateAWSResource(t *testing.T) {
	ucp, ucpClient, cloudcontrolClient := initializeTest(t)

	cloudcontrolClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params *cloudcontrol.GetResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceOutput, error) {
		notfound := types.ResourceNotFoundException{
			Message: to.StringPtr("Resource not found"),
		}
		return nil, &notfound
	})

	cloudcontrolClient.EXPECT().CreateResource(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params *cloudcontrol.CreateResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.CreateResourceOutput, error) {
		output := cloudcontrol.CreateResourceOutput{
			ProgressEvent: &types.ProgressEvent{
				OperationStatus: types.OperationStatusSuccess,
				RequestToken:    to.StringPtr(testAWSRequestToken),
			},
		}
		return &output, nil
	})

	requestBody := map[string]interface{}{
		"properties": map[string]interface{}{
			"RetentionPeriodHours": 178,
			"ShardCount":           3,
		},
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)
	createRequest, err := http.NewRequest("PUT", ucp.URL+basePath+testProxyRequestAWSPath, bytes.NewBuffer(body))
	require.NoError(t, err)
	createResponse, err := ucpClient.httpClient.Do(createRequest)
	require.NoError(t, err)

	responseBody, err := io.ReadAll(createResponse.Body)
	require.NoError(t, err)
	createResponseBody := map[string]interface{}{}
	err = json.Unmarshal(responseBody, &createResponseBody)
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, createResponse.StatusCode)
	expectedResponse := map[string]interface{}{
		"id":   testProxyRequestAWSPath,
		"name": testAWSResourceName,
		"type": testAWSResourceType,
		"properties": map[string]interface{}{
			"RetentionPeriodHours": float64(178),
			"ShardCount":           float64(3),
		},
	}
	assert.Equal(t, expectedResponse, createResponseBody)
	assert.Equal(t, ucp.URL+basePath+testProxyRequestAWSAsyncPath+"/operationStatuses/"+strings.ToLower(testAWSRequestToken), createResponse.Header.Get("Azure-Asyncoperation"), "Azure-Asyncoperation header is not set correctly")
	assert.Equal(t, ucp.URL+basePath+testProxyRequestAWSAsyncPath+"/operationResults/"+strings.ToLower(testAWSRequestToken), createResponse.Header.Get("Location"), "Location header is not set correctly")
}

func initializeTest(t *testing.T) (*httptest.Server, Client, *aws.MockAWSClient) {
	ctrl := gomock.NewController(t)
	cloudcontrolClient := aws.NewMockAWSClient(ctrl)

	router := mux.NewRouter()
	ucp := httptest.NewServer(router)
	ctx := context.Background()
	err := api.Register(ctx, router, controller.Options{
		BasePath:  basePath,
		AWSClient: cloudcontrolClient,
	})
	require.NoError(t, err)

	ucpClient := NewClient(http.DefaultClient, ucp.URL+basePath)

	return ucp, ucpClient, cloudcontrolClient
}
