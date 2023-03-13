// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package aws

// Tests that test with Mock RP functionality and UCP Server

import (
	"context"
	"net/http"
	"testing"

	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/test/testutil"

	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_DeleteAWSResource(t *testing.T) {
	ucp, ucpClient, cloudcontrolClient, _ := initializeTest(t)

	cloudcontrolClient.EXPECT().DeleteResource(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params *cloudcontrol.DeleteResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.DeleteResourceOutput, error) {
		output := cloudcontrol.DeleteResourceOutput{
			ProgressEvent: &types.ProgressEvent{
				OperationStatus: types.OperationStatusSuccess,
				RequestToken:    to.Ptr(testAWSRequestToken),
			},
		}
		return &output, nil
	})

	deleteRequest, err := testutil.GetARMTestHTTPRequestFromURL(context.Background(), http.MethodDelete, ucp.URL+basePath+testProxyRequestAWSPath, nil)
	require.NoError(t, err, "creating request failed")

	ctx := testutil.ARMTestContextFromRequest(deleteRequest)
	deleteRequest = deleteRequest.WithContext(ctx)

	deleteResponse, err := ucpClient.httpClient.Do(deleteRequest)
	require.NoError(t, err)

	assert.Equal(t, http.StatusAccepted, deleteResponse.StatusCode)
}
