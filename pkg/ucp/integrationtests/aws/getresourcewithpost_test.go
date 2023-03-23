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

	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/test/testutil"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_GetAWSResourceWithPost(t *testing.T) {
	ucp, ucpClient, cloudcontrolClient, cloudformationClient := initializeTest(t)

	primaryIdentifiers := map[string]any{
		"primaryIdentifier": []any{
			"/properties/Name",
		},
	}
	serialized, err := json.Marshal(primaryIdentifiers)
	require.NoError(t, err)
	output := cloudformation.DescribeTypeOutput{
		TypeName: aws.String("AWS::Kinesis::Stream"),
		Schema:   to.Ptr(string(serialized)),
	}

	cloudformationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any()).Return(&output, nil)

	getResponseBody := map[string]any{
		"RetentionPeriodHours": 178,
		"ShardCount":           3,
	}
	getResponseBodyBytes, err := json.Marshal(getResponseBody)
	require.NoError(t, err)

	cloudcontrolClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params *cloudcontrol.GetResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceOutput, error) {
		output := cloudcontrol.GetResourceOutput{
			ResourceDescription: &types.ResourceDescription{
				Identifier: to.Ptr(testAWSResourceName),
				Properties: to.Ptr(string(getResponseBodyBytes)),
			},
		}
		return &output, nil
	})

	requestBody := map[string]any{
		"properties": map[string]any{
			"Name":                 "testStream",
			"RetentionPeriodHours": 178,
			"ShardCount":           3,
		},
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	getRequest, err := testutil.GetARMTestHTTPRequestFromURL(context.Background(), http.MethodPost, ucp.URL+basePath+testProxyRequestAWSCollectionPath+"/:get", body)
	require.NoError(t, err, "creating request failed")

	ctx := testutil.ARMTestContextFromRequest(getRequest)
	getRequest = getRequest.WithContext(ctx)

	getResponse, err := ucpClient.httpClient.Do(getRequest)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, getResponse.StatusCode)
}
