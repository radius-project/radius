// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package aws

// Tests that test with Mock RP functionality and UCP Server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_GetAWSResource(t *testing.T) {
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
				Identifier: to.StringPtr(testAWSResourceName),
				Properties: to.StringPtr(string(getResponseBodyBytes)),
			},
		}
		return &output, nil
	})

	getRequest, err := http.NewRequest("GET", ucp.URL+basePath+testProxyRequestAWSPath, nil)
	require.NoError(t, err)
	getResponse, err := ucpClient.httpClient.Do(getRequest)
	require.NoError(t, err)

	responseBody, err := io.ReadAll(getResponse.Body)
	require.NoError(t, err)
	actualResponseBody := map[string]interface{}{}
	err = json.Unmarshal(responseBody, &actualResponseBody)
	require.NoError(t, err)

	expectedResponse := map[string]interface{}{
		"id":   testProxyRequestAWSPath,
		"name": testAWSResourceName,
		"type": testAWSResourceType,
		"properties": map[string]interface{}{
			"RetentionPeriodHours": float64(178),
			"ShardCount":           float64(3),
		},
	}

	assert.Equal(t, http.StatusOK, getResponse.StatusCode)
	assert.Equal(t, expectedResponse, actualResponseBody)
}

func Test_GetAWSResourc_ResourceNotFound(t *testing.T) {
	ucp, ucpClient, cloudcontrolClient := initializeTest(t)

	cloudcontrolClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params *cloudcontrol.GetResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceOutput, error) {
		notfound := types.ResourceNotFoundException{
			Message: to.StringPtr("Resource not found"),
		}
		return nil, &notfound
	})

	getRequest, err := http.NewRequest("GET", ucp.URL+basePath+testProxyRequestAWSPath, nil)
	require.NoError(t, err)
	getResponse, err := ucpClient.httpClient.Do(getRequest)
	require.NoError(t, err)

	responseBody, err := io.ReadAll(getResponse.Body)
	require.NoError(t, err)
	getResponseBody := map[string]interface{}{}
	err = json.Unmarshal(responseBody, &getResponseBody)
	require.NoError(t, err)

	expectedResponse := map[string]interface{}{
		"error": map[string]interface{}{
			"code":    "NotFound",
			"message": fmt.Sprintf("the resource with id '%s' was not found", testProxyRequestAWSPath),
			"target":  testProxyRequestAWSPath,
		},
	}

	assert.Equal(t, http.StatusNotFound, getResponse.StatusCode)
	assert.Equal(t, expectedResponse, getResponseBody)
}
