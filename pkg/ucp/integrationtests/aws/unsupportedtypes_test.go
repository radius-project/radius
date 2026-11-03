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
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	cctypes "github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/pkg/ucp/testhost"
)

// secretValue is a property value that must never appear in an error.
const secretValue = "do-not-echo-this-value"

const awsProvidersPath = "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/"

// sendUCPRequest sends a request through the UCP router and returns the status code, the error message, and the raw
// response body.
func sendUCPRequest(t *testing.T, ucp *testhost.TestHost, method string, path string, body any) (int, string, string) {
	t.Helper()
	var content []byte
	if body != nil {
		var err error
		content, err = json.Marshal(body)
		require.NoError(t, err)
	}
	request, err := rpctest.NewHTTPRequestWithContent(t.Context(), method, ucp.BaseURL()+path, content)
	require.NoError(t, err)
	request = request.WithContext(rpctest.NewARMRequestContext(request))
	request.Header.Set(sdkclients.AWSResourceOwnerHeader, testOwner)

	response, err := ucp.Client().Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	errorResponse := v1.ErrorResponse{}
	message := ""
	if json.Unmarshal(raw, &errorResponse) == nil && errorResponse.Error != nil {
		message = errorResponse.Error.Message
	}
	return response.StatusCode, message, string(raw)
}

// Test_LegacyAWSType_Returns400WithoutCallingAWS sends every AWS resource operation for a legacy type through the UCP
// routes. The AWS mocks have no expected calls, so any call to AWS fails the test.
func Test_LegacyAWSType_Returns400WithoutCallingAWS(t *testing.T) {
	ucp, _, _ := initializeAWSTest(t)

	body := map[string]any{"properties": map[string]any{"PolicyName": "p", "PolicyDocument": secretValue}}
	requests := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodPut, "AWS.IAM/Policy/p", body},
		{http.MethodGet, "AWS.IAM/Policy/p", nil},
		{http.MethodDelete, "AWS.IAM/Policy/p", nil},
		{http.MethodGet, "AWS.IAM/Policy", nil},
		{http.MethodPost, "AWS.IAM/Policy/:put", body},
		{http.MethodPost, "AWS.IAM/Policy/:get", body},
		{http.MethodPost, "AWS.IAM/Policy/:delete", body},
	}
	for _, r := range requests {
		t.Run(r.method+" "+r.path, func(t *testing.T) {
			status, message, raw := sendUCPRequest(t, ucp, r.method, awsProvidersPath+r.path, r.body)
			require.Equal(t, http.StatusBadRequest, status, raw)
			require.Equal(t, "Radius does not support the legacy AWS resource type AWS::IAM::Policy. Use AWS::IAM::ManagedPolicy, or AWS::IAM::RolePolicy, AWS::IAM::UserPolicy, or AWS::IAM::GroupPolicy for an inline policy instead.", message)
			require.NotContains(t, raw, secretValue)
		})
	}
}

// Test_ReadOnlyAWSType_CreateReturns400 creates a read-only type through the UCP routes. UCP calls DescribeType and
// stops before it reads or creates the resource.
func Test_ReadOnlyAWSType_CreateReturns400(t *testing.T) {
	requests := []struct {
		method string
		path   string
	}{
		{http.MethodPut, "AWS.EC2/Image/ami-1"},
		{http.MethodPost, "AWS.EC2/Image/:put"},
	}
	for _, r := range requests {
		t.Run(r.method+" "+r.path, func(t *testing.T) {
			ucp, _, cloudformationClient := initializeAWSTest(t)
			cloudformationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeTypeOutput{
				ProvisioningType: cftypes.ProvisioningTypeNonProvisionable,
				Schema:           new(`{"primaryIdentifier": ["/properties/ImageId"], "handlers": {"read": {}, "list": {}}}`),
			}, nil)

			body := map[string]any{"properties": map[string]any{"ImageId": "ami-1", "Name": secretValue}}
			status, message, raw := sendUCPRequest(t, ucp, r.method, awsProvidersPath+r.path, body)
			require.Equal(t, http.StatusBadRequest, status, raw)
			require.Equal(t, "The AWS resource type AWS::EC2::Image is created by AWS, so Radius cannot create or change it. Reference an existing resource with the Bicep `existing` keyword instead.", message)
			require.NotContains(t, raw, secretValue)
		})
	}
}

// Test_ImmutableAWSType_ChangeReturns400 changes an existing resource of a create-and-delete-only type through the
// UCP routes. UCP names the changed property and never calls UpdateResource.
func Test_ImmutableAWSType_ChangeReturns400(t *testing.T) {
	ucp, cloudcontrolClient, cloudformationClient := initializeAWSTest(t)
	cloudformationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeTypeOutput{
		ProvisioningType: cftypes.ProvisioningTypeImmutable,
		Schema:           new(`{"primaryIdentifier": ["/properties/Name"], "createOnlyProperties": ["/properties/Name", "/properties/Size"], "handlers": {"create": {}, "read": {}, "delete": {}}}`),
	}, nil)
	cloudcontrolClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(&cloudcontrol.GetResourceOutput{
		ResourceDescription: &cctypes.ResourceDescription{Identifier: new("name"), Properties: new(`{"Name": "name", "Size": "small"}`)},
	}, nil)

	body := map[string]any{"properties": map[string]any{"Name": "name", "Size": secretValue}}
	status, message, raw := sendUCPRequest(t, ucp, http.MethodPost, awsProvidersPath+"AWS.Test/Immutable/:put", body)
	require.Equal(t, http.StatusBadRequest, status, raw)
	require.Equal(t, "The AWS resource type AWS::Test::Immutable can only be created and deleted, so these properties of the existing resource cannot change: Size. To change them, delete the resource and deploy again.", message)
	require.NotContains(t, raw, secretValue)
}

// Test_ImmutableAWSType_WriteOnlyChangeReturns400 sends a changed write-only value for an existing resource of a
// create-and-delete-only type through both write routes. AWS never returns write-only values, so UCP cannot confirm
// the value is unchanged and must not report success without applying it. UpdateResource is never called.
func Test_ImmutableAWSType_WriteOnlyChangeReturns400(t *testing.T) {
	requests := []struct {
		method string
		path   string
	}{
		{http.MethodPut, "AWS.Test/Immutable/name"},
		{http.MethodPost, "AWS.Test/Immutable/:put"},
	}
	for _, r := range requests {
		t.Run(r.method+" "+r.path, func(t *testing.T) {
			ucp, cloudcontrolClient, cloudformationClient := initializeAWSTest(t)
			cloudformationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeTypeOutput{
				ProvisioningType: cftypes.ProvisioningTypeImmutable,
				Schema:           new(`{"primaryIdentifier": ["/properties/Name"], "createOnlyProperties": ["/properties/Name", "/properties/Password"], "writeOnlyProperties": ["/properties/Password"], "handlers": {"create": {}, "read": {}, "delete": {}}}`),
			}, nil)
			cloudcontrolClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(&cloudcontrol.GetResourceOutput{
				ResourceDescription: &cctypes.ResourceDescription{Identifier: new("name"), Properties: new(`{"Name": "name"}`)},
			}, nil)

			body := map[string]any{"properties": map[string]any{"Name": "name", "Password": secretValue}}
			status, message, raw := sendUCPRequest(t, ucp, r.method, awsProvidersPath+r.path, body)
			require.Equal(t, http.StatusBadRequest, status, raw)
			require.Equal(t, "The AWS resource type AWS::Test::Immutable can only be created and deleted, and AWS cannot update it in place. Radius cannot confirm that these write-only properties of the existing resource are unchanged, because AWS does not return their values: Password. To change them, delete the resource and deploy again.", message)
			require.NotContains(t, raw, secretValue)
		})
	}
}
