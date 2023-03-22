// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package aws

// Tests that test with Mock RP functionality and UCP Server

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/test/testutil"

	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_GetOperationResults(t *testing.T) {
	ucp, ucpClient, cloudcontrolClient, _ := initializeTest(t)

	cloudcontrolClient.EXPECT().GetResourceRequestStatus(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params *cloudcontrol.GetResourceRequestStatusInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceRequestStatusOutput, error) {
		output := cloudcontrol.GetResourceRequestStatusOutput{
			ProgressEvent: &types.ProgressEvent{
				RequestToken: to.Ptr(testAWSRequestToken),
			},
		}
		return &output, nil
	})

	operationResultsRequest, err := testutil.GetARMTestHTTPRequestFromURL(context.Background(), http.MethodGet, ucp.URL+basePath+testProxyRequestAWSAsyncPath+"/operationResults/"+strings.ToLower(testAWSRequestToken), nil)
	require.NoError(t, err, "creating request failed")

	ctx := testutil.ARMTestContextFromRequest(operationResultsRequest)
	operationResultsRequest = operationResultsRequest.WithContext(ctx)

	operationResultsResponse, err := ucpClient.httpClient.Do(operationResultsRequest)
	require.NoError(t, err)

	assert.Equal(t, http.StatusAccepted, operationResultsResponse.StatusCode)
}
