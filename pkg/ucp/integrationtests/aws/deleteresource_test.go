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
	"strings"
	"testing"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_DeleteAWSResource(t *testing.T) {
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

	cloudcontrolClient.EXPECT().DeleteResource(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params *cloudcontrol.DeleteResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.DeleteResourceOutput, error) {
		output := cloudcontrol.DeleteResourceOutput{
			ProgressEvent: &types.ProgressEvent{
				OperationStatus: types.OperationStatusSuccess,
				RequestToken:    to.StringPtr(testAWSRequestToken),
			},
		}
		return &output, nil
	})

	deleteRequest, err := http.NewRequest("DELETE", ucp.URL+basePath+testProxyRequestAWSPath, nil)
	require.NoError(t, err)
	deleteResponse, err := ucpClient.httpClient.Do(deleteRequest)
	require.NoError(t, err)

	responseBody, err := io.ReadAll(deleteResponse.Body)
	require.NoError(t, err)
	actualResponseBody := map[string]interface{}{}
	err = json.Unmarshal(responseBody, &actualResponseBody)
	require.NoError(t, err)

	assert.Equal(t, http.StatusAccepted, deleteResponse.StatusCode)
	assert.Equal(t, ucp.URL+basePath+testProxyRequestAWSAsyncPath+"/operationStatuses/"+strings.ToLower(testAWSRequestToken), deleteResponse.Header.Get("Azure-Asyncoperation"), "Azure-Asyncoperation header is not set correctly")
	assert.Equal(t, ucp.URL+basePath+testProxyRequestAWSAsyncPath+"/operationResults/"+strings.ToLower(testAWSRequestToken), deleteResponse.Header.Get("Location"), "Location header is not set correctly")
}

func Test_DeleteAWSResource_ResourceDoesNotExist(t *testing.T) {
	ucp, ucpClient, cloudcontrolClient := initializeTest(t)

	cloudcontrolClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params *cloudcontrol.GetResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceOutput, error) {
		notfound := types.ResourceNotFoundException{
			Message: to.StringPtr("Resource not found"),
		}
		return nil, &notfound
	})

	deleteRequest, err := http.NewRequest("DELETE", ucp.URL+basePath+testProxyRequestAWSPath, nil)
	require.NoError(t, err)
	deleteResponse, err := ucpClient.httpClient.Do(deleteRequest)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNoContent, deleteResponse.StatusCode)
}
