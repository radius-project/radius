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
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	requestSecret = "request-secret"
	awsSecret     = "aws-secret"
)

func Test_secretProperties(t *testing.T) {
	t.Parallel()

	schema := `{"writeOnlyProperties": ["/properties/MasterUserPassword", "/properties/Users/*/Password", "Malformed"]}`

	t.Run("write-only properties", func(t *testing.T) {
		t.Parallel()
		paths, err := secretProperties("AWS::RDS::DBInstance", schema)
		require.NoError(t, err)
		require.Equal(t, []string{"MasterUserPassword", "Users/*/Password"}, paths)
	})

	t.Run("adds secrets the schema does not mark", func(t *testing.T) {
		t.Parallel()
		paths, err := secretProperties("aws::ec2::verifiedaccesstrustprovider", `{}`)
		require.NoError(t, err)
		require.Equal(t, []string{"OidcOptions/ClientSecret"}, paths)
	})

	t.Run("invalid schema", func(t *testing.T) {
		t.Parallel()
		_, err := secretProperties("AWS::RDS::DBInstance", `not json`)
		require.Error(t, err)
	})
}

func Test_buildResponseProperties(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current map[string]any
		request map[string]any
		secrets []string
		want    map[string]any
	}{
		{
			name:    "request values take precedence",
			current: map[string]any{"Name": "old", "Arn": "arn"},
			request: map[string]any{"Name": "new"},
			want:    map[string]any{"Name": "new", "Arn": "arn"},
		},
		{
			name:    "top-level secret is not echoed",
			request: map[string]any{"Name": "db", "MasterUserPassword": requestSecret},
			secrets: []string{"MasterUserPassword"},
			want:    map[string]any{"Name": "db"},
		},
		{
			name:    "nested secret is not echoed and AWS value is kept",
			current: map[string]any{"OidcOptions": map[string]any{"Issuer": "old", "ClientSecret": awsSecret, "Scope": "openid"}},
			request: map[string]any{"OidcOptions": map[string]any{"Issuer": "new", "ClientSecret": requestSecret}},
			secrets: []string{"OidcOptions/ClientSecret"},
			want:    map[string]any{"OidcOptions": map[string]any{"Issuer": "new", "ClientSecret": awsSecret, "Scope": "openid"}},
		},
		{
			name: "secret in every array item is not echoed",
			request: map[string]any{"Users": []any{
				map[string]any{"Name": "a", "Password": requestSecret},
				map[string]any{"Name": "b", "Password": requestSecret},
			}},
			secrets: []string{"Users/*/Password"},
			want: map[string]any{"Users": []any{
				map[string]any{"Name": "a"},
				map[string]any{"Name": "b"},
			}},
		},
		{
			name:    "whole array of secrets is not echoed",
			request: map[string]any{"Name": "a", "Keys": []any{requestSecret}},
			secrets: []string{"Keys/*"},
			want:    map[string]any{"Name": "a"},
		},
		{
			name:    "value AWS returns is passed through",
			current: map[string]any{"ClientName": "old", "ClientSecret": awsSecret},
			request: map[string]any{"ClientName": "new"},
			want:    map[string]any{"ClientName": "new", "ClientSecret": awsSecret},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			current := deepCopy(tt.current)
			request := deepCopy(tt.request)

			got := buildResponseProperties(tt.current, tt.request, tt.secrets)
			require.Equal(t, tt.want, got)

			// The inputs are not changed: the request is still sent to AWS as is.
			require.Equal(t, current, deepCopy(tt.current))
			require.Equal(t, request, deepCopy(tt.request))
		})
	}
}

// secretsTestCase is a create or update request through a PUT or POST /:put controller.
type secretsTestCase struct {
	name     string
	resource *AWSTestResource
	// current is what AWS returns for the resource, or nil when it does not exist.
	current map[string]any
	request map[string]any
	// lookup is true when the controller reads the resource before writing it.
	lookup bool
	// write is "create", "update", or "" when nothing changes.
	write          string
	wantStatus     int
	wantProperties map[string]any
}

// Test_CreateOrUpdateAWSResource_Secrets checks that a PUT never echoes a write-only or other secret value from the
// request, and never stores one in the record.
func Test_CreateOrUpdateAWSResource_Secrets(t *testing.T) {
	tests := []secretsTestCase{
		{
			name:     "create does not echo write-only value",
			resource: newRDSDBInstanceTestResource(false),
			request:  map[string]any{"DBInstanceIdentifier": "app-db", "MasterUsername": "admin", "MasterUserPassword": requestSecret},
			lookup:   true,
			write:    "create",

			wantStatus:     http.StatusCreated,
			wantProperties: map[string]any{"DBInstanceIdentifier": "app-db", "MasterUsername": "admin", "provisioningState": "Provisioning"},
		},
		{
			name:     "update does not echo write-only value",
			resource: newRDSDBInstanceTestResource(false),
			current:  map[string]any{"DBInstanceIdentifier": "app-db", "MasterUsername": "admin"},
			request:  map[string]any{"DBInstanceIdentifier": "app-db", "MasterUsername": "admin", "MasterUserPassword": requestSecret},
			lookup:   true,
			write:    "update",

			wantStatus:     http.StatusCreated,
			wantProperties: map[string]any{"DBInstanceIdentifier": "app-db", "MasterUsername": "admin", "provisioningState": "Provisioning"},
		},
		{
			name:     "no change does not echo write-only value",
			resource: newRDSDBInstanceTestResource(true),
			current:  map[string]any{"DBInstanceIdentifier": "app-db", "MasterUserPassword": awsSecret},
			request:  map[string]any{"DBInstanceIdentifier": "app-db", "MasterUserPassword": requestSecret},
			lookup:   true,

			wantStatus:     http.StatusOK,
			wantProperties: map[string]any{"DBInstanceIdentifier": "app-db", "MasterUserPassword": awsSecret, "provisioningState": "Succeeded"},
		},
		{
			name:     "create does not echo secret the schema does not mark",
			resource: newVerifiedAccessTrustProviderTestResource(),
			request:  map[string]any{"TrustProviderType": "user", "OidcOptions": map[string]any{"Issuer": "https://issuer", "ClientSecret": requestSecret}},
			write:    "create",

			wantStatus:     http.StatusCreated,
			wantProperties: map[string]any{"TrustProviderType": "user", "OidcOptions": map[string]any{"Issuer": "https://issuer"}, "provisioningState": "Provisioning"},
		},
		{
			name:     "update keeps the client secret AWS returns",
			resource: newCognitoUserPoolClientTestResource(),
			current:  map[string]any{"UserPoolId": "pool", "ClientId": "client", "ClientName": "old", "ClientSecret": awsSecret},
			request:  map[string]any{"UserPoolId": "pool", "ClientId": "client", "ClientName": "new"},
			lookup:   true,
			write:    "update",

			wantStatus: http.StatusCreated,
			wantProperties: map[string]any{
				"UserPoolId":        "pool",
				"ClientId":          "client",
				"ClientName":        "new",
				"ClientSecret":      awsSecret,
				"provisioningState": "Provisioning",
			},
		},
		{
			name:     "update does not echo secret the schema does not mark and keeps the AWS value",
			resource: newVerifiedAccessTrustProviderTestResource(),
			current:  map[string]any{"VerifiedAccessTrustProviderId": "vatp-123", "OidcOptions": map[string]any{"Issuer": "old", "ClientSecret": awsSecret}},
			request:  map[string]any{"VerifiedAccessTrustProviderId": "vatp-123", "OidcOptions": map[string]any{"Issuer": "new", "ClientSecret": requestSecret}},
			lookup:   true,
			write:    "update",

			wantStatus: http.StatusCreated,
			wantProperties: map[string]any{
				"VerifiedAccessTrustProviderId": "vatp-123",
				"OidcOptions":                   map[string]any{"Issuer": "new", "ClientSecret": awsSecret},
				"provisioningState":             "Provisioning",
			},
		},
		{
			name:     "no change keeps the client secret AWS returns",
			resource: newCognitoUserPoolClientTestResource(),
			current:  map[string]any{"UserPoolId": "pool", "ClientId": "client", "ClientName": "app", "ClientSecret": awsSecret},
			request:  map[string]any{"UserPoolId": "pool", "ClientId": "client", "ClientName": "app"},
			lookup:   true,

			wantStatus: http.StatusOK,
			wantProperties: map[string]any{
				"UserPoolId":        "pool",
				"ClientId":          "client",
				"ClientName":        "app",
				"ClientSecret":      awsSecret,
				"provisioningState": "Succeeded",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runSecretsTestCase(t, tt)
		})
	}
}

func Test_GetAWSResource_PassesThroughAWSSecrets(t *testing.T) {
	testResource := newCognitoUserPoolClientTestResource()
	properties := map[string]any{"UserPoolId": "pool", "ClientId": "client", "ClientSecret": awsSecret}
	propertiesBytes, err := json.Marshal(properties)
	require.NoError(t, err)

	testOptions := setupTest(t)
	saveTestRecord(t, testOptions, testResource.SingleResourcePath, func(rec *datamodel.AWSResource) {
		rec.Properties.AWSIdentifier = testResource.ResourceName
	})
	testOptions.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudcontrol.GetResourceOutput{
			ResourceDescription: &types.ResourceDescription{
				Identifier: aws.String(testResource.ResourceName),
				Properties: aws.String(string(propertiesBytes)),
			},
		}, nil)

	awsController, err := NewGetAWSResource(testOptions.ControllerOptions(), ucp_aws.Clients{
		CloudControl:   testOptions.AWSCloudControlClient,
		CloudFormation: testOptions.AWSCloudFormationClient,
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, testResource.SingleResourcePath, nil)
	require.NoError(t, err)

	status, got := runController(t, awsController, request)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, properties, got)
}

func runSecretsTestCase(t *testing.T, tt secretsTestCase) {
	testOptions := setupTest(t)
	testOptions.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudformation.DescribeTypeOutput{
			TypeName: aws.String(tt.resource.AWSResourceType),
			Schema:   aws.String(tt.resource.Schema),
		}, nil)

	if tt.lookup {
		if tt.current == nil {
			testOptions.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
				nil, &types.ResourceNotFoundException{Message: aws.String("Resource not found")})
		} else {
			current, err := json.Marshal(tt.current)
			require.NoError(t, err)
			testOptions.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
				&cloudcontrol.GetResourceOutput{
					ResourceDescription: &types.ResourceDescription{Properties: aws.String(string(current))},
				}, nil)
		}
	}

	progressEvent := &types.ProgressEvent{
		OperationStatus: types.OperationStatusInProgress,
		RequestToken:    aws.String(testAWSRequestToken),
		Identifier:      aws.String(tt.resource.ResourceName),
	}
	switch tt.write {
	case "create":
		testOptions.AWSCloudControlClient.EXPECT().CreateResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
			&cloudcontrol.CreateResourceOutput{ProgressEvent: progressEvent}, nil)
	case "update":
		testOptions.AWSCloudControlClient.EXPECT().UpdateResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
			&cloudcontrol.UpdateResourceOutput{ProgressEvent: progressEvent}, nil)
	}

	body, err := json.Marshal(map[string]any{"properties": tt.request})
	require.NoError(t, err)

	awsController, err := newTestPutController(testOptions.ControllerOptions(), testOptions.Clients())
	require.NoError(t, err)
	request, err := http.NewRequest(http.MethodPut, tt.resource.SingleResourcePath, bytes.NewBuffer(body))
	require.NoError(t, err)
	request.Host = testHost
	request.URL.Host = testHost
	request.URL.Scheme = testScheme
	withOwner(request)

	status, got := runController(t, awsController, request)
	require.Equal(t, tt.wantStatus, status)
	require.Equal(t, tt.wantProperties, got)

	// The record holds the request's properties without its secrets.
	rec := getTestRecord(t, testOptions, tt.resource.SingleResourcePath)
	require.NotNil(t, rec)
	raw, err := json.Marshal(rec)
	require.NoError(t, err)
	require.NotContains(t, string(raw), requestSecret)
}

// runController runs the controller and returns the response status code and the properties of the response body.
func runController(t *testing.T, awsController armrpc_controller.Controller, request *http.Request) (int, map[string]any) {
	ctx := rpctest.NewARMRequestContext(request)
	response, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	require.NoError(t, response.Apply(ctx, w, request))

	res := w.Result()
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	responseObject := map[string]any{}
	require.NoError(t, json.Unmarshal(body, &responseObject))
	properties, _ := responseObject["properties"].(map[string]any)
	return res.StatusCode, properties
}

// newRDSDBInstanceTestResource returns a DB instance whose MasterUserPassword is write-only. When createOnly is true,
// the password is also create-only, so the patch keeps the current value and a request with only a new password
// changes nothing.
func newRDSDBInstanceTestResource(createOnly bool) *AWSTestResource {
	createOnlyProperties := []any{"/properties/DBInstanceIdentifier"}
	if createOnly {
		createOnlyProperties = append(createOnlyProperties, "/properties/MasterUserPassword")
	}
	schema := map[string]any{
		"primaryIdentifier":    []any{"/properties/DBInstanceIdentifier"},
		"readOnlyProperties":   []any{"/properties/Endpoint/Address"},
		"createOnlyProperties": createOnlyProperties,
		"writeOnlyProperties":  []any{"/properties/MasterUserPassword"},
	}
	return CreateAWSTestResource("AWS.RDS/DBInstance", "AWS::RDS::DBInstance", "app-db", "AWS.RDS",
		"arn:aws:rds:us-west-2:123456789012:db:app-db", schema)
}

// newVerifiedAccessTrustProviderTestResource returns a trust provider whose OidcOptions/ClientSecret holds a secret
// that the schema does not mark write-only.
func newVerifiedAccessTrustProviderTestResource() *AWSTestResource {
	schema := map[string]any{
		"primaryIdentifier":  []any{"/properties/VerifiedAccessTrustProviderId"},
		"readOnlyProperties": []any{"/properties/VerifiedAccessTrustProviderId"},
	}
	return CreateAWSTestResource("AWS.EC2/VerifiedAccessTrustProvider", "AWS::EC2::VerifiedAccessTrustProvider", "vatp-123",
		"AWS.EC2", "arn:aws:ec2:us-west-2:123456789012:verified-access-trust-provider/vatp-123", schema)
}

// newCognitoUserPoolClientTestResource returns a user pool client whose ClientSecret is generated and returned by AWS.
func newCognitoUserPoolClientTestResource() *AWSTestResource {
	schema := map[string]any{
		"primaryIdentifier":    []any{"/properties/UserPoolId", "/properties/ClientId"},
		"readOnlyProperties":   []any{"/properties/ClientId", "/properties/ClientSecret"},
		"createOnlyProperties": []any{"/properties/UserPoolId"},
	}
	return CreateAWSTestResource("AWS.Cognito/UserPoolClient", "AWS::Cognito::UserPoolClient", "pool|client", "AWS.Cognito",
		"arn:aws:cognito-idp:us-west-2:123456789012:userpool/pool", schema)
}
