/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package aws

// Tests that test with Mock RP functionality and UCP Server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/to"

	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testProxyRequestAWSListPath = "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/Stream"

func Test_ListAWSResources(t *testing.T) {
	ucp, _, _, cloudcontrolClient, _ := initializeAWSTest(t)

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
					Identifier: to.Ptr(testAWSResourceName),
					Properties: to.Ptr(string(getResponseBodyBytes)),
				},
			},
		}
		return &output, nil
	})

	listRequest, err := rpctest.NewHTTPRequestWithContent(context.Background(), http.MethodGet, ucp.BaseURL+testProxyRequestAWSListPath, nil)
	require.NoError(t, err, "creating request failed")

	ctx := rpctest.NewARMRequestContext(listRequest)
	listRequest = listRequest.WithContext(ctx)

	listResponse, err := ucp.Client().Do(listRequest)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, listResponse.StatusCode)
}
