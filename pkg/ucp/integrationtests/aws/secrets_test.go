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

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/ucp/testhost"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	testDBInstancePath           = "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.RDS/DBInstance/app-db"
	testDBInstanceCollectionPath = "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.RDS/DBInstance"
	testDBInstanceSchema         = `{
		"primaryIdentifier": ["/properties/DBInstanceIdentifier"],
		"createOnlyProperties": ["/properties/DBInstanceIdentifier"],
		"writeOnlyProperties": ["/properties/MasterUserPassword"]
	}`
	testPassword = "request-password"
)

// Test_PutAWSResource_DoesNotEchoWriteOnlyValue checks through the UCP routes that a write-only value from the request
// is sent to AWS but never returned in the response.
func Test_PutAWSResource_DoesNotEchoWriteOnlyValue(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "PUT", method: http.MethodPut, path: testDBInstancePath},
		{name: "POST /:put", method: http.MethodPost, path: testDBInstanceCollectionPath + "/:put"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ucp, cloudcontrolClient, cloudFormationClient := initializeAWSTest(t)

			cloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeTypeOutput{
				TypeName: aws.String("AWS::RDS::DBInstance"),
				Schema:   aws.String(testDBInstanceSchema),
			}, nil)

			cloudcontrolClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
				nil, &types.ResourceNotFoundException{Message: aws.String("Resource not found")})

			var sent map[string]any
			cloudcontrolClient.EXPECT().CreateResource(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, params *cloudcontrol.CreateResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.CreateResourceOutput, error) {
					require.NoError(t, json.Unmarshal([]byte(aws.ToString(params.DesiredState)), &sent))
					return &cloudcontrol.CreateResourceOutput{
						ProgressEvent: &types.ProgressEvent{
							OperationStatus: types.OperationStatusInProgress,
							RequestToken:    aws.String(testAWSRequestToken),
							Identifier:      aws.String("app-db"),
						},
					}, nil
				})

			status, properties := sendPut(t, ucp, tt.method, tt.path, map[string]any{
				"DBInstanceIdentifier": "app-db",
				"MasterUsername":       "admin",
				"MasterUserPassword":   testPassword,
			})

			require.Equal(t, http.StatusCreated, status)
			require.Equal(t, map[string]any{
				"DBInstanceIdentifier": "app-db",
				"MasterUsername":       "admin",
				"provisioningState":    "Provisioning",
			}, properties)
			require.Equal(t, testPassword, sent["MasterUserPassword"], "the password must still be sent to AWS")
		})
	}
}

// Test_PutAWSResource_NoChange_DoesNotEchoWriteOnlyValue checks the 200 UCP returns when nothing changed.
func Test_PutAWSResource_NoChange_DoesNotEchoWriteOnlyValue(t *testing.T) {
	ucp, cloudcontrolClient, cloudFormationClient := initializeAWSTest(t)

	cloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeTypeOutput{
		TypeName: aws.String("AWS::RDS::DBInstance"),
		// A request with a write-only value changes nothing only when the property is also create-only: the patch
		// keeps the current value.
		Schema: aws.String(`{
			"primaryIdentifier": ["/properties/DBInstanceIdentifier"],
			"createOnlyProperties": ["/properties/DBInstanceIdentifier", "/properties/MasterUserPassword"],
			"writeOnlyProperties": ["/properties/MasterUserPassword"]
		}`),
	}, nil)

	cloudcontrolClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(&cloudcontrol.GetResourceOutput{
		ResourceDescription: &types.ResourceDescription{
			Identifier: aws.String("app-db"),
			Properties: aws.String(`{"DBInstanceIdentifier": "app-db", "MasterUsername": "admin", "MasterUserPassword": "aws-value"}`),
		},
	}, nil)

	status, properties := sendPut(t, ucp, http.MethodPut, testDBInstancePath, map[string]any{
		"DBInstanceIdentifier": "app-db",
		"MasterUsername":       "admin",
		"MasterUserPassword":   testPassword,
	})

	require.Equal(t, http.StatusOK, status)
	require.Equal(t, map[string]any{
		"DBInstanceIdentifier": "app-db",
		"MasterUsername":       "admin",
		"MasterUserPassword":   "aws-value",
		"provisioningState":    "Succeeded",
	}, properties)
}

// sendPut sends a create or update request to UCP and returns the response status code and properties.
func sendPut(t *testing.T, ucp *testhost.TestHost, method string, path string, properties map[string]any) (int, map[string]any) {
	body, err := json.Marshal(map[string]any{"properties": properties})
	require.NoError(t, err)

	request, err := rpctest.NewHTTPRequestWithContent(t.Context(), method, ucp.BaseURL()+path, body)
	require.NoError(t, err)
	request = request.WithContext(rpctest.NewARMRequestContext(request))

	response, err := ucp.Client().Do(request)
	require.NoError(t, err)
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NotContains(t, string(responseBody), testPassword)

	responseObject := map[string]any{}
	require.NoError(t, json.Unmarshal(responseBody, &responseObject))
	got, _ := responseObject["properties"].(map[string]any)
	return response.StatusCode, got
}
