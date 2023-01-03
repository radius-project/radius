// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package aws

// Tests that test with Mock RP functionality and UCP Server

import (
	"context"
	"encoding/json"
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
	ucp, ucpClient, cloudcontrolClient, _ := initializeTest(t)

	getResponseBody := map[string]any{
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

	listRequest, err := http.NewRequest(http.MethodGet, ucp.URL+basePath+testProxyRequestAWSListPath, nil)
	require.NoError(t, err)
	listResponse, err := ucpClient.httpClient.Do(listRequest)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, listResponse.StatusCode)
}
