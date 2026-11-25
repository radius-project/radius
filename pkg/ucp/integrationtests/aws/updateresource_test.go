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
	"encoding/json"
	"net/http"
	"testing"

	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"

	"github.com/radius-project/radius/pkg/armrpc/rpctest"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// Test_UpdateAWSResource_UnchangedRedeploy redeploys a log group whose tags AWS reports in a different order and whose
// DataProtectionPolicy AWS reports as an empty object. UCP must not call UpdateResource.
func Test_UpdateAWSResource_UnchangedRedeploy(t *testing.T) {
	ucp, cloudcontrolClient, cloudFormationClient := initializeAWSTest(t)

	const schema = `{
		"typeName": "AWS::Logs::LogGroup",
		"properties": {
			"LogGroupName": {"type": "string"},
			"RetentionInDays": {"type": "integer"},
			"DataProtectionPolicy": {"type": "object"},
			"Arn": {"type": "string"},
			"Tags": {"type": "array", "insertionOrder": false, "uniqueItems": true, "items": {"$ref": "#/definitions/Tag"}}
		},
		"definitions": {"Tag": {"type": "object", "properties": {"Key": {"type": "string"}, "Value": {"type": "string"}}}},
		"readOnlyProperties": ["/properties/Arn"],
		"createOnlyProperties": ["/properties/LogGroupName"],
		"primaryIdentifier": ["/properties/LogGroupName"]
	}`
	cloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeTypeOutput{
		TypeName: aws.String("AWS::Logs::LogGroup"),
		Schema:   aws.String(schema),
	}, nil)

	current, err := json.Marshal(map[string]any{
		"LogGroupName":         "my-log-group",
		"RetentionInDays":      7,
		"DataProtectionPolicy": map[string]any{},
		"Arn":                  "arn:aws:logs:us-east-1:1234567:log-group:my-log-group:*",
		"Tags":                 []any{map[string]any{"Key": "b", "Value": "2"}, map[string]any{"Key": "a", "Value": "1"}},
	})
	require.NoError(t, err)
	cloudcontrolClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(&cloudcontrol.GetResourceOutput{
		ResourceDescription: &types.ResourceDescription{Properties: aws.String(string(current))},
	}, nil)

	body, err := json.Marshal(map[string]any{
		"properties": map[string]any{
			"LogGroupName":    "my-log-group",
			"RetentionInDays": 7,
			"Tags":            []any{map[string]any{"Key": "a", "Value": "1"}, map[string]any{"Key": "b", "Value": "2"}},
		},
	})
	require.NoError(t, err)

	request, err := rpctest.NewHTTPRequestWithContent(t.Context(), http.MethodPut, ucp.BaseURL()+"/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Logs/LogGroup/my-log-group", body)
	require.NoError(t, err, "creating request failed")
	request = request.WithContext(rpctest.NewARMRequestContext(request))
	request.Header.Set(sdkclients.AWSResourceOwnerHeader, testOwner)

	response, err := ucp.Client().Do(request)
	require.NoError(t, err)
	defer response.Body.Close()

	require.Equal(t, http.StatusOK, response.StatusCode)
}
