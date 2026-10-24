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
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	cctypes "github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"
	"go.uber.org/mock/gomock"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/retry"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
)

// secretValue is a property value that must never appear in an error.
const secretValue = "do-not-echo-this-value"

type controllerFactory func(armrpc_controller.Options, ucp_aws.Clients) (armrpc_controller.Controller, error)

// runController runs a controller for a request to path and returns the status code and the error message.
func runController(t *testing.T, factory controllerFactory, clients ucp_aws.Clients, method string, path string, body map[string]any) (int, string, string) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewBuffer(b)
	}
	request, err := http.NewRequest(method, path, reader)
	require.NoError(t, err)
	request.Host = testHost
	request.URL.Host = testHost
	request.URL.Scheme = testScheme

	controller, err := factory(armrpc_controller.Options{}, clients)
	require.NoError(t, err)
	ctx := rpctest.NewARMRequestContext(request)
	response, err := controller.Run(ctx, nil, request)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	require.NoError(t, response.Apply(ctx, w, request))
	res := w.Result()
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	message := ""
	errorResponse := v1.ErrorResponse{}
	if json.Unmarshal(raw, &errorResponse) == nil && errorResponse.Error != nil {
		message = errorResponse.Error.Message
	}
	return res.StatusCode, message, string(raw)
}

func clientsFor(options TestOptions) ucp_aws.Clients {
	return ucp_aws.Clients{CloudControl: options.AWSCloudControlClient, CloudFormation: options.AWSCloudFormationClient}
}

// Test_UnsupportedTypes_FailWithoutCallingAWS checks every AWS resource operation for a legacy and a deprecated type.
// The mocks have no expected calls, so any call to AWS fails the test.
func Test_UnsupportedTypes_FailWithoutCallingAWS(t *testing.T) {
	types := []struct {
		resourceType string
		want         string
	}{
		{
			resourceType: "AWS.IAM/Policy",
			want:         "Radius does not support the legacy AWS resource type AWS::IAM::Policy. Use AWS::IAM::ManagedPolicy, or AWS::IAM::RolePolicy, AWS::IAM::UserPolicy, or AWS::IAM::GroupPolicy for an inline policy instead.",
		},
		{
			resourceType: "AWS.OpsWorks/Stack",
			want:         "Radius does not support the AWS resource type AWS::OpsWorks::Stack: AWS OpsWorks reached end of life.",
		},
	}
	body := map[string]any{"properties": map[string]any{"PolicyName": "p", "Secret": secretValue}}
	operations := []struct {
		name    string
		factory controllerFactory
		method  string
		suffix  string
		body    map[string]any
	}{
		{"PUT", NewCreateOrUpdateAWSResource, http.MethodPut, "/name", body},
		{"POST /:put", NewCreateOrUpdateAWSResourceWithPost, http.MethodPost, "", body},
		{"GET", NewGetAWSResource, http.MethodGet, "/name", nil},
		{"POST /:get", func(o armrpc_controller.Options, c ucp_aws.Clients) (armrpc_controller.Controller, error) {
			return NewGetAWSResourceWithPost(o, c, retry.NewNoOpRetryer())
		}, http.MethodPost, "", body},
		{"DELETE", NewDeleteAWSResource, http.MethodDelete, "/name", nil},
		{"POST /:delete", NewDeleteAWSResourceWithPost, http.MethodPost, "", body},
		{"LIST", NewListAWSResources, http.MethodGet, "", nil},
	}
	for _, tt := range types {
		for _, op := range operations {
			t.Run(tt.resourceType+" "+op.name, func(t *testing.T) {
				options := setupTest(t)
				path := "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/" + tt.resourceType + op.suffix
				status, message, raw := runController(t, op.factory, clientsFor(options), op.method, path, op.body)
				require.Equal(t, http.StatusBadRequest, status)
				require.Equal(t, tt.want, message)
				require.NotContains(t, raw, secretValue)
			})
		}
	}
}

// nonProvisionableSchema is a NON_PROVISIONABLE schema. A read-only type has read and list handlers; a type that Cloud
// Control cannot create has none.
func nonProvisionableSchema(t *testing.T, handlers ...string) string {
	h := map[string]any{}
	for _, name := range handlers {
		h[name] = map[string]any{"permissions": []string{}}
	}
	b, err := json.Marshal(map[string]any{
		"primaryIdentifier": []string{"/properties/Name"},
		"handlers":          h,
	})
	require.NoError(t, err)
	return string(b)
}

func Test_Create_NonProvisionableTypes(t *testing.T) {
	tests := []struct {
		name     string
		handlers []string
		want     string
	}{
		{
			name:     "read-only",
			handlers: []string{"read", "list"},
			want:     "The AWS resource type AWS::EC2::Image is created by AWS, so Radius cannot create or change it. Reference an existing resource with the Bicep `existing` keyword instead.",
		},
		{
			name: "not creatable",
			want: "Radius cannot create the AWS resource type AWS::EC2::Image because AWS Cloud Control does not support it.",
		},
	}
	creates := []struct {
		name    string
		factory controllerFactory
		method  string
		suffix  string
	}{
		{"PUT", NewCreateOrUpdateAWSResource, http.MethodPut, "/name"},
		{"POST /:put", NewCreateOrUpdateAWSResourceWithPost, http.MethodPost, ""},
	}
	for _, tt := range tests {
		for _, op := range creates {
			t.Run(tt.name+" "+op.name, func(t *testing.T) {
				options := setupTest(t)
				options.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeTypeOutput{
					ProvisioningType: cftypes.ProvisioningTypeNonProvisionable,
					Schema:           aws.String(nonProvisionableSchema(t, tt.handlers...)),
				}, nil)
				options.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
					nil, &cctypes.ResourceNotFoundException{Message: aws.String("not found")})

				path := "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/AWS.EC2/Image" + op.suffix
				body := map[string]any{"properties": map[string]any{"Name": "name", "Secret": secretValue}}
				status, message, raw := runController(t, op.factory, clientsFor(options), op.method, path, body)
				require.Equal(t, http.StatusBadRequest, status)
				require.Equal(t, tt.want, message)
				require.NotContains(t, raw, secretValue)
			})
		}
	}
}

// immutableSchema is the schema of a create-and-delete-only type with a write-only Password property.
const immutableSchema = `{
	"primaryIdentifier": ["/properties/Name"],
	"readOnlyProperties": ["/properties/Arn"],
	"createOnlyProperties": ["/properties/Name", "/properties/Size", "/properties/Settings", "/properties/Password"],
	"writeOnlyProperties": ["/properties/Password"],
	"handlers": {"create": {}, "read": {}, "delete": {}, "list": {}}
}`

func Test_Update_ImmutableType(t *testing.T) {
	current := `{"Name": "name", "Arn": "arn:aws:x:us-west-2:1:y/name", "Size": 1, "Settings": {"Mode": "a"}}`
	tests := []struct {
		name       string
		properties map[string]any
		// want is the error message, or "" when the deploy succeeds without calling AWS.
		want string
	}{
		{
			name:       "changed properties",
			properties: map[string]any{"Name": "name", "Size": 2, "Settings": map[string]any{"Mode": secretValue}, "Password": secretValue},
			want:       "The AWS resource type AWS::Test::Immutable can only be created and deleted, so these properties of the existing resource cannot change: Settings, Size. To change them, delete the resource and deploy again.",
		},
		{
			name:       "no changes",
			properties: map[string]any{"Name": "name", "Size": 1},
		},
		{
			name:       "only write-only properties",
			properties: map[string]any{"Name": "name", "Size": 1, "Password": secretValue},
		},
	}
	updates := []struct {
		name    string
		factory controllerFactory
		method  string
		suffix  string
	}{
		{"PUT", NewCreateOrUpdateAWSResource, http.MethodPut, "/name"},
		{"POST /:put", NewCreateOrUpdateAWSResourceWithPost, http.MethodPost, ""},
	}
	for _, tt := range tests {
		for _, op := range updates {
			t.Run(tt.name+" "+op.name, func(t *testing.T) {
				options := setupTest(t)
				options.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeTypeOutput{
					ProvisioningType: cftypes.ProvisioningTypeImmutable,
					Schema:           aws.String(immutableSchema),
				}, nil)
				options.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(&cloudcontrol.GetResourceOutput{
					ResourceDescription: &cctypes.ResourceDescription{Identifier: aws.String("name"), Properties: aws.String(current)},
				}, nil)

				path := "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/AWS.Test/Immutable" + op.suffix
				status, message, raw := runController(t, op.factory, clientsFor(options), op.method, path, map[string]any{"properties": tt.properties})
				if tt.want == "" {
					require.Equal(t, http.StatusOK, status, raw)
					return
				}
				require.Equal(t, http.StatusBadRequest, status)
				require.Equal(t, tt.want, message)
				require.NotContains(t, raw, secretValue)
			})
		}
	}
}

func Test_Create_ImmutableType(t *testing.T) {
	options := setupTest(t)
	options.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeTypeOutput{
		ProvisioningType: cftypes.ProvisioningTypeImmutable,
		Schema:           aws.String(immutableSchema),
	}, nil)
	options.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil, &cctypes.ResourceNotFoundException{Message: aws.String("not found")})
	options.AWSCloudControlClient.EXPECT().CreateResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(&cloudcontrol.CreateResourceOutput{
		ProgressEvent: &cctypes.ProgressEvent{RequestToken: aws.String(testAWSRequestToken), Identifier: aws.String("name")},
	}, nil)

	path := "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/AWS.Test/Immutable"
	status, _, raw := runController(t, NewCreateOrUpdateAWSResourceWithPost, clientsFor(options), http.MethodPost, path, map[string]any{"properties": map[string]any{"Name": "name", "Size": 1}})
	require.Equal(t, http.StatusCreated, status, raw)
}

func Test_Update_ReadOnlyType(t *testing.T) {
	options := setupTest(t)
	options.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeTypeOutput{
		ProvisioningType: cftypes.ProvisioningTypeNonProvisionable,
		Schema:           aws.String(nonProvisionableSchema(t, "read", "list")),
	}, nil)
	options.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(&cloudcontrol.GetResourceOutput{
		ResourceDescription: &cctypes.ResourceDescription{Identifier: aws.String("name"), Properties: aws.String(`{"Name": "name", "Description": "a"}`)},
	}, nil)

	path := "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/AWS.EC2/Image/name"
	status, message, raw := runController(t, NewCreateOrUpdateAWSResource, clientsFor(options), http.MethodPut, path, map[string]any{"properties": map[string]any{"Name": "name", "Description": secretValue}})
	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, "The AWS resource type AWS::EC2::Image is created by AWS, so Radius cannot create or change it. Reference an existing resource with the Bicep `existing` keyword instead.", message)
	require.NotContains(t, raw, secretValue)
}

func Test_CheckPatch_FullyMutable(t *testing.T) {
	patch := jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "/Size", Value: 2}}
	for _, provisioningType := range []cftypes.ProvisioningType{cftypes.ProvisioningTypeFullyMutable, ""} {
		got, errResponse, err := checkPatch("AWS::Test::Mutable", &cloudformation.DescribeTypeOutput{ProvisioningType: provisioningType}, patch)
		require.NoError(t, err)
		require.Nil(t, errResponse)
		require.Equal(t, patch, got)
	}
}

func Test_TopLevelProperty(t *testing.T) {
	require.Equal(t, "Size", topLevelProperty("/Size"))
	require.Equal(t, "Settings", topLevelProperty("/Settings/Mode"))
	require.Equal(t, "Tags", topLevelProperty("/Tags/0/Value"))
	require.Equal(t, "a/b~c", topLevelProperty("/a~1b~0c/d"))
}

func Test_IsWriteOnly(t *testing.T) {
	schema := &typeSchema{WriteOnlyProperties: []string{"/properties/Password", "/properties/Auth/Token"}}
	require.True(t, schema.isWriteOnly("/Password"))
	require.True(t, schema.isWriteOnly("/Auth/Token"))
	require.True(t, schema.isWriteOnly("/Auth/Token/Value"))
	require.False(t, schema.isWriteOnly("/PasswordLength"))
	require.False(t, schema.isWriteOnly("/Auth"))
	require.False(t, schema.isWriteOnly("/Auth/Type"))
}
