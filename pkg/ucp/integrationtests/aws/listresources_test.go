// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package aws

// Tests that test with Mock RP functionality and UCP Server

import (
	"context"
	"encoding/json"
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

const testProxyRequestAWSListPath = "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/Stream"

func Test_ListAWSResources(t *testing.T) {
	ucp, ucpClient, cloudcontrolClient := initializeTest(t)

	getResponseBody := map[string]interface{}{
		"RetentionPeriodHours": 178,
		"ShardCount":           3,
	}
	getResponseBodyBytes, err := json.Marshal(getResponseBody)
	require.NoError(t, err)

	cloudcontrolClient.EXPECT().ListResources(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params *cloudcontrol.ListResourcesInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.ListResourcesOutput, error) {
		output := cloudcontrol.ListResourcesOutput{
			ResourceDescriptions: []types.ResourceDescription{
				{
					Identifier: to.StringPtr(testAWSResourceName),
					Properties: to.StringPtr(string(getResponseBodyBytes)),
				},
			},
		}
		return &output, nil
	})

	listRequest, err := http.NewRequest("GET", ucp.URL+basePath+testProxyRequestAWSListPath, nil)
	require.NoError(t, err)
	listResponse, err := ucpClient.httpClient.Do(listRequest)
	require.NoError(t, err)

	responseBody, err := io.ReadAll(listResponse.Body)
	require.NoError(t, err)
	actualResponseBody := map[string]interface{}{}
	err = json.Unmarshal(responseBody, &actualResponseBody)
	require.NoError(t, err)

	expectedResponse := map[string]interface{}{
		"value": []interface{}{
			map[string]interface{}{
				"id":   testProxyRequestAWSListPath,
				"name": testAWSResourceName,
				"type": testAWSResourceType,
				"properties": map[string]interface{}{
					"RetentionPeriodHours": float64(178),
					"ShardCount":           float64(3),
				},
			},
		},
	}

	assert.Equal(t, http.StatusOK, listResponse.StatusCode)
	assert.Equal(t, expectedResponse, actualResponseBody)
}
