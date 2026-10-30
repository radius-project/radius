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
	"errors"
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
	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

// secretValue is a property value that must never appear in an error.
const secretValue = "do-not-echo-this-value"

type controllerFactory func(armrpc_controller.Options, ucp_aws.Clients) (armrpc_controller.Controller, error)

// runTypeCheckRequest runs a controller for a request to path and returns the status code and the error message.
func runTypeCheckRequest(t *testing.T, factory controllerFactory, options TestOptions, method string, path string, body map[string]any) (int, string, string) {
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
	withOwner(request)

	controller, err := factory(options.ControllerOptions(), options.Clients())
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
		{"PUT", newTestPutController, http.MethodPut, "/name", body},
		{"GET", NewGetAWSResource, http.MethodGet, "/name", nil},
		{"POST /:get", func(o armrpc_controller.Options, c ucp_aws.Clients) (armrpc_controller.Controller, error) {
			return NewGetAWSResourceWithPost(o, c, retry.NewNoOpRetryer())
		}, http.MethodPost, "", body},
		{"DELETE", NewDeleteAWSResource, http.MethodDelete, "/name", nil},
		{"LIST", NewListAWSResources, http.MethodGet, "", nil},
	}
	for _, tt := range types {
		for _, op := range operations {
			t.Run(tt.resourceType+" "+op.name, func(t *testing.T) {
				options := setupTest(t)
				path := "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/" + tt.resourceType + op.suffix
				status, message, raw := runTypeCheckRequest(t, op.factory, options, op.method, path, op.body)
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

// Test_Put_NonProvisionableTypes checks that UCP rejects a PUT of a NON_PROVISIONABLE type without reading the resource
// from Cloud Control, which rejects that read for most of these types.
func Test_Put_NonProvisionableTypes(t *testing.T) {
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
			want: "Radius cannot create or change the AWS resource type AWS::EC2::Image because AWS Cloud Control does not support it.",
		},
	}
	puts := []struct {
		name    string
		factory controllerFactory
		method  string
		suffix  string
	}{
		{"PUT", newTestPutController, http.MethodPut, "/name"},
	}
	for _, tt := range tests {
		for _, op := range puts {
			t.Run(tt.name+" "+op.name, func(t *testing.T) {
				options := setupTest(t)
				options.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeTypeOutput{
					ProvisioningType: cftypes.ProvisioningTypeNonProvisionable,
					Schema:           aws.String(nonProvisionableSchema(t, tt.handlers...)),
				}, nil)

				path := "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/AWS.EC2/Image" + op.suffix
				body := map[string]any{"properties": map[string]any{"Name": "name", "Secret": secretValue}}
				status, message, raw := runTypeCheckRequest(t, op.factory, options, op.method, path, body)
				require.Equal(t, http.StatusBadRequest, status)
				require.Equal(t, tt.want, message)
				require.NotContains(t, raw, secretValue)
			})
		}
	}
}

// immutableSchema is the schema of a create-and-delete-only type with write-only properties at the top level, inside
// an object, and inside array items.
const immutableSchema = `{
	"primaryIdentifier": ["/properties/Name"],
	"readOnlyProperties": ["/properties/Arn"],
	"createOnlyProperties": ["/properties/Name", "/properties/Size", "/properties/Settings", "/properties/Password"],
	"writeOnlyProperties": ["/properties/Password", "/properties/Auth/Token", "/properties/Users/*/Secret"],
	"handlers": {"create": {}, "read": {}, "delete": {}, "list": {}}
}`

// Test_Update_ImmutableType takes over an existing resource of a create-and-delete-only type. UCP has no fingerprints
// for it yet, so a write-only value is not sent and does not fail the deploy, because UCP cannot tell whether it
// changed. A changed readable property fails. Test_WriteOnly_ImmutableType covers write-only values with fingerprints.
func Test_Update_ImmutableType(t *testing.T) {
	current := `{"Name": "name", "Arn": "arn:aws:x:us-west-2:1:y/name", "Size": 1, "Settings": {"Mode": "a"}, "Users": [{"Name": "u"}]}`
	tests := []struct {
		name       string
		properties map[string]any
		// want is the error message, or "" when the deploy succeeds without calling AWS. No case calls UpdateResource,
		// and the mock fails the test if one does.
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
			name:       "write-only property",
			properties: map[string]any{"Name": "name", "Size": 1, "Password": secretValue},
		},
		{
			// AWS omits Auth because its only property is write-only, so the patch adds the whole object.
			name:       "write-only property in an absent parent",
			properties: map[string]any{"Name": "name", "Size": 1, "Auth": map[string]any{"Token": secretValue}},
		},
		{
			name:       "readable property next to a write-only property in an absent parent",
			properties: map[string]any{"Name": "name", "Size": 1, "Auth": map[string]any{"Token": secretValue, "Type": secretValue}},
			want:       "The AWS resource type AWS::Test::Immutable can only be created and deleted, so these properties of the existing resource cannot change: Auth. To change them, delete the resource and deploy again.",
		},
		{
			name:       "write-only property in array items",
			properties: map[string]any{"Name": "name", "Size": 1, "Users": []any{map[string]any{"Name": "u", "Secret": secretValue}}},
		},
		{
			name:       "several write-only properties",
			properties: map[string]any{"Name": "name", "Size": 1, "Users": []any{map[string]any{"Name": "u", "Secret": secretValue}}, "Password": secretValue},
		},
	}
	updates := []struct {
		name    string
		factory controllerFactory
		method  string
		suffix  string
	}{
		{"PUT", newTestPutController, http.MethodPut, "/name"},
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
				status, message, raw := runTypeCheckRequest(t, op.factory, options, op.method, path, map[string]any{"properties": tt.properties})
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

	path := "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/AWS.Test/Immutable/name"
	status, _, raw := runTypeCheckRequest(t, newTestPutController, options, http.MethodPut, path, map[string]any{"properties": map[string]any{"Name": "name", "Size": 1}})
	require.Equal(t, http.StatusCreated, status, raw)
}

func Test_CheckPatch_FullyMutable(t *testing.T) {
	patch := jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "/Size", Value: 2}}
	for _, provisioningType := range []cftypes.ProvisioningType{cftypes.ProvisioningTypeFullyMutable, ""} {
		require.Nil(t, checkPatch("AWS::Test::Mutable", &cloudformation.DescribeTypeOutput{ProvisioningType: provisioningType}, patch))
	}
}

func Test_TopLevelProperty(t *testing.T) {
	require.Equal(t, "Size", topLevelProperty("/Size"))
	require.Equal(t, "Settings", topLevelProperty("/Settings/Mode"))
	require.Equal(t, "Tags", topLevelProperty("/Tags/0/Value"))
	require.Equal(t, "a/b~c", topLevelProperty("/a~1b~0c/d"))
}

func Test_WriteOnlyPattern(t *testing.T) {
	schema := &typeSchema{WriteOnlyProperties: []string{"/properties/Password", "/properties/Auth/Token", "/properties/Users/*/Secret"}}
	tests := []struct {
		path string
		want string
	}{
		{"/Password", "Password"},
		{"/Auth/Token", "Auth/Token"},
		{"/Auth/Token/Value", "Auth/Token"},
		{"/Users/0/Secret", "Users/*/Secret"},
		{"/Users/12/Secret/Value", "Users/*/Secret"},
		{"/PasswordLength", ""},
		{"/Auth", ""},
		{"/Auth/Type", ""},
		{"/Users/0/Name", ""},
		{"/Users/0", ""},
	}
	for _, tt := range tests {
		got, ok := schema.writeOnlyPattern(tt.path)
		require.Equal(t, tt.want, got, tt.path)
		require.Equal(t, tt.want != "", ok, tt.path)
	}
}

func Test_LeafPaths(t *testing.T) {
	got := leafPaths("/Auth", map[string]any{
		"Token":   "x",
		"a/b~c":   1,
		"Empty":   map[string]any{},
		"List":    []any{"y", map[string]any{"Secret": "z"}},
		"NoItems": []any{},
	})
	require.ElementsMatch(t, []string{"/Auth/Token", "/Auth/a~1b~0c", "/Auth/Empty", "/Auth/List/0", "/Auth/List/1/Secret", "/Auth/NoItems"}, got)
	require.Equal(t, []string{"/Size"}, leafPaths("/Size", 2))
}

// Test_InvalidSchema checks that a schema UCP cannot read fails a PUT before UCP reads the resource from AWS, instead
// of letting it through. "handlers" that is not an object passes the secrets check but not the type check, which a
// PUT of any type needs for its write-only properties.
func Test_InvalidSchema(t *testing.T) {
	tests := []struct {
		name             string
		provisioningType cftypes.ProvisioningType
		schema           string
	}{
		{name: "NON_PROVISIONABLE", provisioningType: cftypes.ProvisioningTypeNonProvisionable, schema: "not json"},
		{
			name:             "IMMUTABLE",
			provisioningType: cftypes.ProvisioningTypeImmutable,
			schema:           `{"primaryIdentifier": ["/properties/Name"], "handlers": ["create"]}`,
		},
	}
	puts := []struct {
		name    string
		factory controllerFactory
		method  string
		suffix  string
	}{
		{"PUT", newTestPutController, http.MethodPut, "/name"},
	}
	for _, tt := range tests {
		for _, op := range puts {
			t.Run(tt.name+" "+op.name, func(t *testing.T) {
				options := setupTest(t)
				options.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeTypeOutput{
					ProvisioningType: tt.provisioningType,
					Schema:           aws.String(tt.schema),
				}, nil)

				path := "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/AWS.Test/Type" + op.suffix
				request, err := http.NewRequest(op.method, path, bytes.NewBufferString(`{"properties": {"Name": "name", "Size": 2}}`))
				require.NoError(t, err)
				controller, err := op.factory(options.ControllerOptions(), options.Clients())
				withOwner(request)
				require.NoError(t, err)
				_, err = controller.Run(rpctest.NewARMRequestContext(request), nil, request)
				require.ErrorContains(t, err, "failed to read the CloudFormation schema")
			})
		}
	}
}

// Test_Put_DescribeTypeError checks that PUT stops before it reads the resource when DescribeType fails.
func Test_Put_DescribeTypeError(t *testing.T) {
	describeErr := errors.New("describe failed")
	puts := []struct {
		name    string
		factory controllerFactory
		method  string
		suffix  string
	}{
		{"PUT", newTestPutController, http.MethodPut, "/name"},
	}
	for _, op := range puts {
		t.Run(op.name, func(t *testing.T) {
			options := setupTest(t)
			options.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, describeErr)

			path := "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/AWS.Test/Type" + op.suffix
			request, err := http.NewRequest(op.method, path, bytes.NewBufferString(`{"properties": {"Name": "name"}}`))
			require.NoError(t, err)
			controller, err := op.factory(options.ControllerOptions(), options.Clients())
			withOwner(request)
			require.NoError(t, err)
			_, err = controller.Run(rpctest.NewARMRequestContext(request), nil, request)
			require.ErrorIs(t, err, describeErr)
		})
	}
}

// Test_ReadOnlyType_GetCallsCloudControl checks that read-only types are only blocked on PUT. A GET still reads from
// Cloud Control, which is how `existing` references them.
func Test_ReadOnlyType_GetCallsCloudControl(t *testing.T) {
	options := setupTest(t)
	saveTestRecord(t, options, "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/AWS.EC2/Image/ami-1", func(rec *datamodel.AWSResource) {
		rec.Properties.AWSIdentifier = "ami-1"
	})
	options.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(&cloudcontrol.GetResourceOutput{
		ResourceDescription: &cctypes.ResourceDescription{Identifier: aws.String("ami-1"), Properties: aws.String(`{"ImageId": "ami-1"}`)},
	}, nil)

	path := "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/AWS.EC2/Image/ami-1"
	status, _, raw := runTypeCheckRequest(t, NewGetAWSResource, options, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, status, raw)
}
