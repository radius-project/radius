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
	"net/http"
	"testing"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_CreateAWSResource(t *testing.T) {
	ucp, ucpClient, cloudcontrolClient, _ := initializeTest(t)

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

	requestBody := map[string]any{
		"properties": map[string]any{
			"RetentionPeriodHours": 178,
			"ShardCount":           3,
		},
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)
	createRequest, err := http.NewRequest(http.MethodPut, ucp.URL+basePath+testProxyRequestAWSPath, bytes.NewBuffer(body))
	require.NoError(t, err)
	createResponse, err := ucpClient.httpClient.Do(createRequest)
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, createResponse.StatusCode)
}
