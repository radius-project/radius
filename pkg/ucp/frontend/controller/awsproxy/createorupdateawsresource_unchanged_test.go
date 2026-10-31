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

package awsproxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// Test_UpdateAWSResource_ValuesAWSReportsDifferently covers redeploys of templates whose values AWS reports in a
// different form: reordered unordered arrays, values AWS normalizes, and empty objects. Neither controller may call
// UpdateResource; the mock fails the test if it is called.
func Test_UpdateAWSResource_ValuesAWSReportsDifferently(t *testing.T) {
	const logGroupSchema = `{
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
	const dbSubnetGroupSchema = `{
		"typeName": "AWS::RDS::DBSubnetGroup",
		"properties": {
			"DBSubnetGroupName": {"type": "string"},
			"DBSubnetGroupDescription": {"type": "string"},
			"SubnetIds": {"type": "array", "uniqueItems": false, "insertionOrder": false, "items": {"type": "string"}}
		},
		"createOnlyProperties": ["/properties/DBSubnetGroupName"],
		"propertyTransform": {"/properties/DBSubnetGroupName": "$lowercase(DBSubnetGroupName)"},
		"primaryIdentifier": ["/properties/DBSubnetGroupName"]
	}`

	tests := []struct {
		name         string
		awsType      string
		resourceType string
		resourceName string
		schema       string
		current      map[string]any
		desired      map[string]any
	}{
		{
			name:         "log group with reordered tags and an empty object",
			awsType:      "AWS::Logs::LogGroup",
			resourceType: "AWS.Logs/LogGroup",
			resourceName: "my-log-group",
			schema:       logGroupSchema,
			current: map[string]any{
				"LogGroupName":         "my-log-group",
				"RetentionInDays":      7,
				"DataProtectionPolicy": map[string]any{},
				"Arn":                  "arn:aws:logs:us-west-2:123456789012:log-group:my-log-group:*",
				"Tags":                 []any{map[string]any{"Key": "b", "Value": "2"}, map[string]any{"Key": "a", "Value": "1"}},
			},
			desired: map[string]any{
				"LogGroupName":    "my-log-group",
				"RetentionInDays": 7,
				"Tags":            []any{map[string]any{"Key": "a", "Value": "1"}, map[string]any{"Key": "b", "Value": "2"}},
			},
		},
		{
			name:         "DB subnet group with a mixed-case name and reordered subnets",
			awsType:      "AWS::RDS::DBSubnetGroup",
			resourceType: "AWS.RDS/DBSubnetGroup",
			resourceName: "app-db",
			schema:       dbSubnetGroupSchema,
			current: map[string]any{
				"DBSubnetGroupName":        "app-db",
				"DBSubnetGroupDescription": "db",
				"SubnetIds":                []any{"subnet-b", "subnet-a"},
			},
			desired: map[string]any{
				"DBSubnetGroupName":        "App-DB",
				"DBSubnetGroupDescription": "db",
				"SubnetIds":                []any{"subnet-a", "subnet-b"},
			},
		},
	}

	collectionPath := func(resourceType string) string {
		return "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/" + resourceType
	}

	for _, tt := range tests {
		for _, withPost := range []bool{false, true} {
			name := tt.name + " (PUT)"
			if withPost {
				name = tt.name + " (POST :put)"
			}
			t.Run(name, func(t *testing.T) {
				testOptions := setupTest(t)
				testOptions.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).Return(
					&cloudformation.DescribeTypeOutput{TypeName: aws.String(tt.awsType), Schema: aws.String(tt.schema)}, nil)

				currentBytes, err := json.Marshal(tt.current)
				require.NoError(t, err)
				testOptions.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
					&cloudcontrol.GetResourceOutput{
						ResourceDescription: &types.ResourceDescription{Properties: aws.String(string(currentBytes))},
					}, nil)

				body, err := json.Marshal(map[string]any{"properties": tt.desired})
				require.NoError(t, err)

				controller, err := NewCreateOrUpdateAWSResource(testOptions.ControllerOptions(), testOptions.Clients())
				require.NoError(t, err)
				request, err := http.NewRequest(http.MethodPut, collectionPath(tt.resourceType)+"/"+tt.resourceName, bytes.NewBuffer(body))
				if withPost {
					controller, err = NewCreateOrUpdateAWSResourceWithPost(testOptions.ControllerOptions(), testOptions.Clients())
					require.NoError(t, err)
					request, err = http.NewRequest(http.MethodPost, collectionPath(tt.resourceType), bytes.NewBuffer(body))
				}
				require.NoError(t, err)
				withOwner(request)

				ctx := rpctest.NewARMRequestContext(request)
				response, err := controller.Run(ctx, nil, request)
				require.NoError(t, err)

				w := httptest.NewRecorder()
				require.NoError(t, response.Apply(ctx, w, request))
				res := w.Result()
				defer res.Body.Close()
				require.Equal(t, http.StatusOK, res.StatusCode)
			})
		}
	}
}
