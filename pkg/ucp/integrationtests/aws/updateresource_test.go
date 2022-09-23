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
	"strings"
	"testing"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const ZeroAWSRequestToken = "00000000-0000-0000-0000-000000000000"

func Test_UpdateAWSResource(t *testing.T) {
	ucp, ucpClient, cloudcontrolClient := initializeTest(t)

	getResponseBody := map[string]interface{}{
		"RetentionPeriodHours": 178,
		"ShardCount":           3,
	}
	getResponseBodyBytes, err := json.Marshal(getResponseBody)
	require.NoError(t, err)

	cloudcontrolClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params *cloudcontrol.GetResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceOutput, error) {
		output := cloudcontrol.GetResourceOutput{
			ResourceDescription: &types.ResourceDescription{
				Properties: to.StringPtr(string(getResponseBodyBytes)),
			},
		}
		return &output, nil
	})

	cloudcontrolClient.EXPECT().UpdateResource(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params *cloudcontrol.UpdateResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.UpdateResourceOutput, error) {
		output := cloudcontrol.UpdateResourceOutput{
			ProgressEvent: &types.ProgressEvent{
				OperationStatus: types.OperationStatusSuccess,
				RequestToken:    to.StringPtr(testAWSRequestToken),
			},
		}
		return &output, nil
	})

	requestBody := map[string]interface{}{
		"properties": map[string]interface{}{
			"RetentionPeriodHours": 180,
			"ShardCount":           4,
		},
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)
	updateRequest, err := http.NewRequest("PUT", ucp.URL+basePath+testProxyRequestAWSPath, bytes.NewBuffer(body))
	require.NoError(t, err)
	updateResponse, err := ucpClient.httpClient.Do(updateRequest)
	require.NoError(t, err)

	responseBody, err := io.ReadAll(updateResponse.Body)
	require.NoError(t, err)
	updateResponseBody := map[string]interface{}{}
	err = json.Unmarshal(responseBody, &updateResponseBody)
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, updateResponse.StatusCode)
	expectedResponse := map[string]interface{}{
		"id":   testProxyRequestAWSPath,
		"name": testAWSResourceName,
		"type": testAWSResourceType,
		"properties": map[string]interface{}{
			"RetentionPeriodHours": float64(180),
			"ShardCount":           float64(4),
		},
	}
	assert.Equal(t, expectedResponse, updateResponseBody)
	assert.Equal(t, ucp.URL+basePath+testProxyRequestAWSAsyncPath+"/operationStatuses/"+strings.ToLower(testAWSRequestToken), updateResponse.Header.Get("Azure-Asyncoperation"), "Azure-Asyncoperation header is not set correctly")
	assert.Equal(t, ucp.URL+basePath+testProxyRequestAWSAsyncPath+"/operationResults/"+strings.ToLower(testAWSRequestToken), updateResponse.Header.Get("Location"), "Location header is not set correctly")
}

func Test_UpdateAWSResource_NoChangeInRequestBody(t *testing.T) {
	ucp, ucpClient, cloudcontrolClient := initializeTest(t)

	getResponseBody := map[string]interface{}{
		"RetentionPeriodHours": 178,
		"ShardCount":           3,
	}
	getResponseBodyBytes, err := json.Marshal(getResponseBody)
	require.NoError(t, err)

	cloudcontrolClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params *cloudcontrol.GetResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceOutput, error) {
		output := cloudcontrol.GetResourceOutput{
			ResourceDescription: &types.ResourceDescription{
				Properties: to.StringPtr(string(getResponseBodyBytes)),
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
	updateRequest, err := http.NewRequest("PUT", ucp.URL+basePath+testProxyRequestAWSPath, bytes.NewBuffer(body))
	require.NoError(t, err)
	updateResponse, err := ucpClient.httpClient.Do(updateRequest)
	require.NoError(t, err)

	responseBody, err := io.ReadAll(updateResponse.Body)
	require.NoError(t, err)
	updateResponseBody := map[string]interface{}{}
	err = json.Unmarshal(responseBody, &updateResponseBody)
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, updateResponse.StatusCode)
	expectedResponse := map[string]interface{}{
		"id":   testProxyRequestAWSPath,
		"name": testAWSResourceName,
		"type": testAWSResourceType,
		"properties": map[string]interface{}{
			"RetentionPeriodHours": float64(178),
			"ShardCount":           float64(3),
		},
	}
	assert.Equal(t, expectedResponse, updateResponseBody)
	// No Update Request is actually made to AWS, so the request token is set to 0
	assert.Equal(t, ucp.URL+basePath+testProxyRequestAWSAsyncPath+"/operationStatuses/"+ZeroAWSRequestToken, updateResponse.Header.Get("Azure-Asyncoperation"), "Azure-Asyncoperation header is not set correctly")
	assert.Equal(t, ucp.URL+basePath+testProxyRequestAWSAsyncPath+"/operationResults/"+ZeroAWSRequestToken, updateResponse.Header.Get("Location"), "Location header is not set correctly")
}
