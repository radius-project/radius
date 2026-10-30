/*
Copyright 2026 The Radius Authors.

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
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/retry"
	ucpaws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// Identifier metadata from DescribeType in us-west-2 on 2026-10-26.
const imageReadSchema = `{
	"properties":{"ImageId":{"type":"string"},"Arn":{"type":"string","description":"The Amazon Resource Name (ARN) of the AMI."}},
	"primaryIdentifier":["/properties/Arn"],
	"additionalIdentifiers":[["/properties/ImageId"]]
}`

const sessionReadSchema = `{
	"primaryIdentifier":["/properties/Arn"],
	"additionalIdentifiers":[["/properties/WorkGroup","/properties/SessionId"]]
}`

const modelReadSchema = `{
	"properties":{"ModelArn":{"type":"string","pattern":"^arn:aws(-[^:]+)?:bedrock:[a-z0-9-]{1,20}::foundation-model/[a-z0-9-]{1,63}[.]{1}([a-z0-9-]{1,63}[.]){0,2}[a-z0-9-]{1,63}([:][a-z0-9-]{1,63}){0,2}$"}},
	"primaryIdentifier":["/properties/ModelArn"],
	"additionalIdentifiers":[["/properties/ModelId"]]
}`

func Test_GetAWSResourceWithPost_ImageID(t *testing.T) {
	t.Parallel()
	testOptions := setupTest(t)
	const identifier = "arn:aws:ec2:us-west-2::image/ami-0829b74b2a8c619b7"
	testOptions.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&cloudformation.DescribeTypeOutput{Schema: aws.String(imageReadSchema)}, nil)
	testOptions.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *cloudcontrol.GetResourceInput, opts ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceOutput, error) {
			require.Equal(t, "AWS::EC2::Image", aws.ToString(input.TypeName))
			require.JSONEq(t, `{"ImageId":"ami-0829b74b2a8c619b7"}`, aws.ToString(input.Identifier))
			options := cloudcontrol.Options{}
			for _, opt := range opts {
				opt(&options)
			}
			require.Equal(t, "us-west-2", options.Region)
			return &cloudcontrol.GetResourceOutput{ResourceDescription: &types.ResourceDescription{
				Identifier: aws.String(identifier),
				Properties: aws.String(`{"ImageId":"ami-0829b74b2a8c619b7","Arn":"` + identifier + `"}`),
			}}, nil
		})
	controller, err := NewGetAWSResourceWithPost(armrpc_controller.Options{}, ucpaws.Clients{
		CloudFormation: testOptions.AWSCloudFormationClient, CloudControl: testOptions.AWSCloudControlClient,
	}, retry.NewNoOpRetryer())
	require.NoError(t, err)
	path := "/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/AWS.EC2/Image"
	req := httptest.NewRequest(http.MethodPost, path+"/:get", bytes.NewBufferString(`{"properties":{"ImageId":"ami-0829b74b2a8c619b7"}}`))
	ctx := rpctest.NewARMRequestContext(req)
	response, err := controller.Run(ctx, nil, req)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	require.NoError(t, response.Apply(ctx, w, req))
	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, path+"/"+identifier, body["id"])
	require.Equal(t, identifier, body["name"])
	require.Equal(t, map[string]any{"ImageId": "ami-0829b74b2a8c619b7", "Arn": identifier}, body["properties"])
}

func Test_ResolveReadIdentifier(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, schema, resourceType, region, identifier string
		properties                                     map[string]any
		status                                         int
	}{
		{
			name: "primary ARN takes precedence", schema: imageReadSchema, resourceType: "AWS::EC2::Image",
			properties: map[string]any{"Arn": "provided-arn", "ImageId": "ami-123"},
			identifier: "provided-arn",
		},
		{
			name: "composite primary identifier", schema: `{"primaryIdentifier":["/properties/Cluster","/properties/Account"]}`,
			properties: map[string]any{"Cluster": "cluster", "Account": "account"},
			identifier: "cluster|account",
		},
		{
			name: "legacy primary without additional metadata", schema: `{"primaryIdentifier":["/properties/Name"]}`,
			properties: map[string]any{"Name": "existing-name"}, identifier: "existing-name",
		},
		{name: "missing identifiers", schema: imageReadSchema, properties: map[string]any{}, status: http.StatusBadRequest},
		{name: "incomplete additional set", schema: sessionReadSchema, properties: map[string]any{"SessionId": "session"}, status: http.StatusBadRequest},
		{name: "null primary", schema: imageReadSchema, properties: map[string]any{"Arn": nil}, status: http.StatusBadRequest},
		{name: "numeric primary", schema: imageReadSchema, properties: map[string]any{"Arn": 12}, status: http.StatusBadRequest},
		{name: "numeric additional", schema: imageReadSchema, properties: map[string]any{"ImageId": 12}, status: http.StatusBadRequest},
		{name: "invalid schema", schema: `{`, status: http.StatusBadRequest},
		{name: "no primary metadata", schema: `{}`, status: http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			controller := &GetAWSResourceWithPost{}
			id, resource, response, err := controller.resolveReadIdentifier(t.Context(), tt.properties, tt.schema, tt.resourceType, tt.region)
			require.NoError(t, err)
			require.Nil(t, resource)
			require.Equal(t, tt.identifier, id)
			if tt.status == 0 {
				require.Nil(t, response)
			} else {
				req := httptest.NewRequest(http.MethodPost, "/", nil)
				w := httptest.NewRecorder()
				require.NoError(t, response.Apply(t.Context(), w, req))
				require.Equal(t, tt.status, w.Code)
			}
		})
	}
}

func Test_ResolveReadIdentifier_List(t *testing.T) {
	t.Parallel()
	const arn = "arn:aws:bedrock:us-west-2::foundation-model/amazon.titan-embed-text-v2:0"
	matching := types.ResourceDescription{Identifier: aws.String(arn), Properties: aws.String(`{"ModelId":"amazon.titan-embed-text-v2:0"}`)}
	other := types.ResourceDescription{Identifier: aws.String("other-arn"), Properties: aws.String(`{"ModelId":"other"}`)}
	for _, tt := range []struct {
		name          string
		pages         []cloudcontrol.ListResourcesOutput
		hydrated      *cloudcontrol.GetResourceOutput
		hydrationErr  error
		listErr       error
		noListOutput  bool
		hydrate       bool
		status        int
		errorContains string
	}{
		{name: "regex is not a template", pages: []cloudcontrol.ListResourcesOutput{{ResourceDescriptions: []types.ResourceDescription{matching}}}},
		{name: "no match", pages: []cloudcontrol.ListResourcesOutput{{ResourceDescriptions: []types.ResourceDescription{other}}}, status: http.StatusNotFound},
		{name: "empty list", pages: []cloudcontrol.ListResourcesOutput{{}}, status: http.StatusNotFound},
		{
			name: "match on later page", pages: []cloudcontrol.ListResourcesOutput{
				{ResourceDescriptions: []types.ResourceDescription{other}, NextToken: aws.String("page2")},
				{ResourceDescriptions: []types.ResourceDescription{matching}},
			},
		},
		{
			name: "ambiguous across pages", pages: []cloudcontrol.ListResourcesOutput{
				{ResourceDescriptions: []types.ResourceDescription{matching}, NextToken: aws.String("page2")},
				{ResourceDescriptions: []types.ResourceDescription{{Identifier: aws.String("second-arn"), Properties: matching.Properties}}},
			}, status: http.StatusConflict,
		},
		{
			name: "duplicate identifier is one resource", pages: []cloudcontrol.ListResourcesOutput{
				{ResourceDescriptions: []types.ResourceDescription{matching}, NextToken: aws.String("page2")},
				{ResourceDescriptions: []types.ResourceDescription{matching}},
			},
		},
		{
			name: "properties absent from list", pages: []cloudcontrol.ListResourcesOutput{{ResourceDescriptions: []types.ResourceDescription{{Identifier: matching.Identifier}}}},
			hydrated: &cloudcontrol.GetResourceOutput{ResourceDescription: &matching},
		},
		{
			name: "properties incomplete in list", pages: []cloudcontrol.ListResourcesOutput{{ResourceDescriptions: []types.ResourceDescription{{Identifier: matching.Identifier, Properties: aws.String(`{"ModelArn":"arn"}`)}}}},
			hydrated: &cloudcontrol.GetResourceOutput{ResourceDescription: &matching},
		},
		{
			name: "resource deleted during list", pages: []cloudcontrol.ListResourcesOutput{{ResourceDescriptions: []types.ResourceDescription{{Identifier: matching.Identifier}}}},
			hydrationErr: &types.ResourceNotFoundException{}, status: http.StatusNotFound,
		},
		{
			name: "read access denied is not a miss", pages: []cloudcontrol.ListResourcesOutput{{ResourceDescriptions: []types.ResourceDescription{{Identifier: matching.Identifier}}}},
			hydrationErr: errors.New("access denied"), errorContains: "access denied",
		},
		{
			name: "missing read identifier is not a match", pages: []cloudcontrol.ListResourcesOutput{{ResourceDescriptions: []types.ResourceDescription{{Identifier: matching.Identifier}}}},
			hydrated: &cloudcontrol.GetResourceOutput{ResourceDescription: &types.ResourceDescription{Properties: aws.String(`{}`)}},
			status:   http.StatusNotFound,
		},
		{
			name: "invalid read identifier is an error", pages: []cloudcontrol.ListResourcesOutput{{ResourceDescriptions: []types.ResourceDescription{{Identifier: matching.Identifier}}}},
			hydrated:      &cloudcontrol.GetResourceOutput{ResourceDescription: &types.ResourceDescription{Properties: aws.String(`{"ModelId":42}`)}},
			errorContains: "must be a string",
		},
		{
			name: "invalid read JSON is an error", pages: []cloudcontrol.ListResourcesOutput{{ResourceDescriptions: []types.ResourceDescription{{Identifier: matching.Identifier}}}},
			hydrated:      &cloudcontrol.GetResourceOutput{ResourceDescription: &types.ResourceDescription{Properties: aws.String(`{`)}},
			errorContains: "decoding Cloud Control read properties",
		},
		{name: "list error", pages: []cloudcontrol.ListResourcesOutput{{}}, listErr: errors.New("list denied"), errorContains: "list denied"},
		{name: "missing list response", pages: []cloudcontrol.ListResourcesOutput{{}}, noListOutput: true, errorContains: "ListResources returned no response"},
		{
			name: "missing read response", pages: []cloudcontrol.ListResourcesOutput{{ResourceDescriptions: []types.ResourceDescription{{Identifier: matching.Identifier}}}},
			hydrate: true, errorContains: "GetResource returned no properties",
		},
		{
			name: "missing resource description", pages: []cloudcontrol.ListResourcesOutput{{ResourceDescriptions: []types.ResourceDescription{{Identifier: matching.Identifier}}}},
			hydrated: &cloudcontrol.GetResourceOutput{}, errorContains: "GetResource returned no properties",
		},
		{
			name: "missing read properties", pages: []cloudcontrol.ListResourcesOutput{{ResourceDescriptions: []types.ResourceDescription{{Identifier: matching.Identifier}}}},
			hydrated: &cloudcontrol.GetResourceOutput{ResourceDescription: &types.ResourceDescription{Identifier: matching.Identifier}}, errorContains: "GetResource returned no properties",
		},
		{name: "invalid list JSON", pages: []cloudcontrol.ListResourcesOutput{{ResourceDescriptions: []types.ResourceDescription{{Identifier: matching.Identifier, Properties: aws.String(`{`)}}}}, errorContains: "decoding"},
		{name: "empty list identifier", pages: []cloudcontrol.ListResourcesOutput{{ResourceDescriptions: []types.ResourceDescription{{Properties: matching.Properties}}}}, errorContains: "empty identifier"},
		{
			name: "pagination loop", pages: []cloudcontrol.ListResourcesOutput{
				{NextToken: aws.String("same")}, {NextToken: aws.String("same")},
			}, errorContains: "repeated pagination token",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			options := setupTest(t)
			for i := range tt.pages {
				page := tt.pages[i]
				options.AWSCloudControlClient.EXPECT().ListResources(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, input *cloudcontrol.ListResourcesInput, opts ...func(*cloudcontrol.Options)) (*cloudcontrol.ListResourcesOutput, error) {
						require.Equal(t, "AWS::Bedrock::FoundationModel", aws.ToString(input.TypeName))
						var model map[string]any
						require.NoError(t, json.Unmarshal([]byte(*input.ResourceModel), &model))
						require.Equal(t, map[string]any{"ModelId": "amazon.titan-embed-text-v2:0"}, model)
						if i == 0 {
							require.Nil(t, input.NextToken)
						} else {
							require.Equal(t, aws.ToString(tt.pages[i-1].NextToken), aws.ToString(input.NextToken))
						}
						o := cloudcontrol.Options{}
						for _, opt := range opts {
							opt(&o)
						}
						require.Equal(t, "us-west-2", o.Region)
						if tt.noListOutput {
							return nil, tt.listErr
						}
						return &page, tt.listErr
					})
			}
			if tt.hydrate || tt.hydrated != nil || tt.hydrationErr != nil {
				options.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, input *cloudcontrol.GetResourceInput, _ ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceOutput, error) {
						require.Equal(t, arn, aws.ToString(input.Identifier))
						return tt.hydrated, tt.hydrationErr
					})
			}
			controller := &GetAWSResourceWithPost{awsClients: ucpaws.Clients{CloudControl: options.AWSCloudControlClient}}
			id, response, err := controller.findReadIdentifier(t.Context(), map[string]any{"ModelId": "amazon.titan-embed-text-v2:0"}, []string{"/properties/ModelId"}, []string{"amazon.titan-embed-text-v2:0"}, "AWS::Bedrock::FoundationModel", "us-west-2")
			if tt.errorContains != "" {
				require.ErrorContains(t, err, tt.errorContains)
				require.Empty(t, id)
				require.Nil(t, response)
				return
			}
			require.NoError(t, err)
			if tt.status != 0 {
				require.Empty(t, id)
				require.NotNil(t, response)
				w := httptest.NewRecorder()
				require.NoError(t, response.Apply(t.Context(), w, httptest.NewRequest(http.MethodPost, "/", nil)))
				require.Equal(t, tt.status, w.Code)
				return
			}
			require.Equal(t, arn, id)
			require.Nil(t, response)
		})
	}
}

func Test_ResolveReadIdentifier_CompositeAdditionalIdentifier(t *testing.T) {
	t.Parallel()
	options := setupTest(t)
	options.AWSCloudControlClient.EXPECT().ListResources(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *cloudcontrol.ListResourcesInput, _ ...func(*cloudcontrol.Options)) (*cloudcontrol.ListResourcesOutput, error) {
			require.JSONEq(t, `{"WorkGroup":"workgroup","SessionId":"session"}`, *input.ResourceModel)
			return &cloudcontrol.ListResourcesOutput{ResourceDescriptions: []types.ResourceDescription{
				{Identifier: aws.String("wrong"), Properties: aws.String(`{"WorkGroup":"other","SessionId":"session"}`)},
				{Identifier: aws.String("matching"), Properties: aws.String(`{"WorkGroup":"workgroup","SessionId":"session"}`)},
			}}, nil
		})
	controller := &GetAWSResourceWithPost{awsClients: ucpaws.Clients{CloudControl: options.AWSCloudControlClient}}
	id, response, err := controller.findReadIdentifier(t.Context(), map[string]any{"WorkGroup": "workgroup", "SessionId": "session"}, []string{"/properties/WorkGroup", "/properties/SessionId"}, []string{"workgroup", "session"}, "AWS::Athena::Session", "us-west-2")
	require.NoError(t, err)
	require.Nil(t, response)
	require.Equal(t, "matching", id)
}

func Test_ResolveReadIdentifier_AlternativeIdentifierSets(t *testing.T) {
	t.Parallel()
	options := setupTest(t)
	options.AWSCloudControlClient.EXPECT().ListResources(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&cloudcontrol.ListResourcesOutput{ResourceDescriptions: []types.ResourceDescription{
			{Identifier: aws.String("unrelated"), Properties: aws.String(`{"Parent":"parent","Child":"child"}`)},
			{Identifier: aws.String("matching"), Properties: aws.String(`{"Name":"name"}`)},
		}}, nil)
	options.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), &cloudcontrol.GetResourceInput{
		TypeName: aws.String("AWS::Example::Resource"), Identifier: aws.String("unrelated"),
	}, gomock.Any()).
		DoAndReturn(func(_ context.Context, input *cloudcontrol.GetResourceInput, _ ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceOutput, error) {
			require.Equal(t, "unrelated", aws.ToString(input.Identifier))
			return &cloudcontrol.GetResourceOutput{ResourceDescription: &types.ResourceDescription{
				Identifier: aws.String("unrelated"), Properties: aws.String(`{"Parent":"parent","Child":"child"}`),
			}}, nil
		})
	options.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *cloudcontrol.GetResourceInput, _ ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceOutput, error) {
			require.JSONEq(t, `{"Name":"name"}`, aws.ToString(input.Identifier))
			return nil, &types.UnsupportedActionException{}
		})
	controller := &GetAWSResourceWithPost{awsClients: ucpaws.Clients{CloudControl: options.AWSCloudControlClient}, retryer: retry.NewNoOpRetryer()}
	schema := `{"primaryIdentifier":["/properties/Arn"],"additionalIdentifiers":[["/properties/Parent","/properties/Child"],["/properties/Name"]]}`
	id, resource, response, err := controller.resolveReadIdentifier(t.Context(), map[string]any{"Name": "name", "Parent": "parent"}, schema, "AWS::Example::Resource", "us-west-2")
	require.NoError(t, err)
	require.Nil(t, resource)
	require.Nil(t, response)
	require.Equal(t, "matching", id)
}

func Test_IdentifierValues(t *testing.T) {
	t.Parallel()
	values, err := identifierValues(map[string]any{"Parent": map[string]any{"a/b~c": "value"}}, []string{"/properties/Parent/a~1b~0c"})
	require.NoError(t, err)
	require.Equal(t, []string{"value"}, values)
	_, err = identifierValues(map[string]any{}, []string{"invalid"})
	require.Error(t, err)
}

func Test_ResolveReadIdentifier_Native(t *testing.T) {
	t.Parallel()
	const arn = "arn:aws:bedrock:us-west-2::foundation-model/model"
	for _, tt := range []struct {
		name, schema, identifier, errorContains string
		properties                              map[string]any
		output                                  *cloudcontrol.GetResourceOutput
		err                                     error
		status                                  int
	}{
		{
			name: "secondary ignores unrelated properties", schema: modelReadSchema,
			properties: map[string]any{"ModelId": "model", "Other": "ignored"}, identifier: `{"ModelId":"model"}`,
			output: &cloudcontrol.GetResourceOutput{ResourceDescription: &types.ResourceDescription{Identifier: aws.String(arn)}},
		},
		{
			name: "composite secondary", schema: sessionReadSchema,
			properties: map[string]any{"WorkGroup": "group", "SessionId": "session"}, identifier: `{"WorkGroup":"group","SessionId":"session"}`,
			output: &cloudcontrol.GetResourceOutput{ResourceDescription: &types.ResourceDescription{Identifier: aws.String(arn)}},
		},
		{
			name: "nested escaped properties", schema: `{"primaryIdentifier":["/properties/Arn"],"additionalIdentifiers":[["/properties/Parent/a~1b~0c","/properties/Parent/Other"]]}`,
			properties: map[string]any{"Parent": map[string]any{"a/b~c": "value", "Other": "other", "Ignored": "ignored"}},
			identifier: `{"Parent":{"a/b~c":"value","Other":"other"}}`,
			output:     &cloudcontrol.GetResourceOutput{ResourceDescription: &types.ResourceDescription{Identifier: aws.String(arn)}},
		},
		{name: "not found does not list", err: &types.ResourceNotFoundException{}, status: http.StatusNotFound},
		{name: "invalid request does not list", err: &types.InvalidRequestException{Message: aws.String("invalid secondary")}, errorContains: "invalid secondary"},
		{name: "access error does not list", err: errors.New("access denied"), errorContains: "access denied"},
		{name: "nil output", errorContains: "no primary identifier"},
		{name: "nil description", output: &cloudcontrol.GetResourceOutput{}, errorContains: "no primary identifier"},
		{name: "missing canonical identifier", output: &cloudcontrol.GetResourceOutput{ResourceDescription: &types.ResourceDescription{}}, errorContains: "no primary identifier"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.schema == "" {
				tt.schema = modelReadSchema
				tt.properties = map[string]any{"ModelId": "model"}
				tt.identifier = `{"ModelId":"model"}`
			}
			options := setupTest(t)
			options.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, input *cloudcontrol.GetResourceInput, opts ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceOutput, error) {
					require.Equal(t, "AWS::Example::Resource", aws.ToString(input.TypeName))
					require.JSONEq(t, tt.identifier, aws.ToString(input.Identifier))
					config := cloudcontrol.Options{}
					for _, opt := range opts {
						opt(&config)
					}
					require.Equal(t, "us-west-2", config.Region)
					return tt.output, tt.err
				})
			controller := &GetAWSResourceWithPost{awsClients: ucpaws.Clients{CloudControl: options.AWSCloudControlClient}, retryer: retry.NewNoOpRetryer()}
			id, resource, response, err := controller.resolveReadIdentifier(t.Context(), tt.properties, tt.schema, "AWS::Example::Resource", "us-west-2")
			if tt.errorContains != "" {
				require.ErrorContains(t, err, tt.errorContains)
				require.Empty(t, id)
				require.Nil(t, resource)
				require.Nil(t, response)
			} else if tt.status != 0 {
				require.NoError(t, err)
				require.Empty(t, id)
				require.Nil(t, resource)
				require.NotNil(t, response)
				w := httptest.NewRecorder()
				require.NoError(t, response.Apply(t.Context(), w, httptest.NewRequest(http.MethodPost, "/", nil)))
				require.Equal(t, tt.status, w.Code)
			} else {
				require.NoError(t, err)
				require.Nil(t, response)
				require.Equal(t, arn, id)
				require.Same(t, tt.output, resource)
			}
		})
	}
}

func Test_ResolveReadIdentifier_NonObjectParent(t *testing.T) {
	t.Parallel()
	controller := &GetAWSResourceWithPost{}
	schema := `{"primaryIdentifier":["/properties/Parent/Name"]}`
	id, _, response, err := controller.resolveReadIdentifier(t.Context(), map[string]any{"Parent": "not-an-object"}, schema, "AWS::Example::Resource", "us-west-2")
	require.NoError(t, err)
	require.Empty(t, id)
	require.NotNil(t, response)
	w := httptest.NewRecorder()
	require.NoError(t, response.Apply(t.Context(), w, httptest.NewRequest(http.MethodPost, "/", nil)))
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "Parent/Name")
}

func Test_ResolveReadIdentifier_NativeRetry(t *testing.T) {
	t.Parallel()
	options := setupTest(t)
	const arn = "arn:aws:bedrock:us-west-2::foundation-model/model"
	output := &cloudcontrol.GetResourceOutput{ResourceDescription: &types.ResourceDescription{Identifier: aws.String(arn)}}
	gomock.InOrder(
		options.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &types.ResourceNotFoundException{}),
		options.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(output, nil),
	)
	controller := &GetAWSResourceWithPost{awsClients: ucpaws.Clients{CloudControl: options.AWSCloudControlClient}, retryer: retry.NewDefaultRetryer()}
	id, resource, response, err := controller.resolveReadIdentifier(t.Context(), map[string]any{"ModelId": "model"}, modelReadSchema, "AWS::Bedrock::FoundationModel", "us-west-2")
	require.NoError(t, err)
	require.Nil(t, response)
	require.Equal(t, arn, id)
	require.Same(t, output, resource)
}

func Test_ResolveReadIdentifier_UnencodableProperties(t *testing.T) {
	t.Parallel()
	options := setupTest(t)
	controller := &GetAWSResourceWithPost{awsClients: ucpaws.Clients{CloudControl: options.AWSCloudControlClient}}
	id, response, err := controller.findReadIdentifier(t.Context(), map[string]any{
		"ModelId": "model", "Invalid": make(chan string),
	}, []string{"/properties/ModelId"}, []string{"model"}, "AWS::Bedrock::FoundationModel", "us-west-2")
	var unsupported *json.UnsupportedTypeError
	require.ErrorAs(t, err, &unsupported)
	require.Empty(t, id)
	require.Nil(t, response)
}

func Test_GetAWSResourceWithPost_IdentifierResolutionErrors(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name          string
		output        *cloudformation.DescribeTypeOutput
		listErr       error
		status        int
		errorContains string
	}{
		{name: "missing describe response", errorContains: "DescribeType returned no schema"},
		{name: "missing schema", output: &cloudformation.DescribeTypeOutput{}, errorContains: "DescribeType returned no schema"},
		{name: "missing identifier", output: &cloudformation.DescribeTypeOutput{Schema: aws.String(imageReadSchema)}, status: http.StatusBadRequest},
		{name: "list failure", output: &cloudformation.DescribeTypeOutput{Schema: aws.String(modelReadSchema)}, listErr: errors.New("list denied"), errorContains: "list denied"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			options := setupTest(t)
			options.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).Return(tt.output, nil)
			if tt.listErr != nil {
				options.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &types.UnsupportedActionException{})
				options.AWSCloudControlClient.EXPECT().ListResources(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, tt.listErr)
			}
			controller, err := NewGetAWSResourceWithPost(armrpc_controller.Options{}, ucpaws.Clients{
				CloudFormation: options.AWSCloudFormationClient, CloudControl: options.AWSCloudControlClient,
			}, retry.NewNoOpRetryer())
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, "/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/AWS.Bedrock/FoundationModel/:get", bytes.NewBufferString(`{"properties":{"ModelId":"model"}}`))
			ctx := rpctest.NewARMRequestContext(req)
			response, err := controller.Run(ctx, nil, req)
			if tt.errorContains != "" {
				require.ErrorContains(t, err, tt.errorContains)
				require.Nil(t, response)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, response)
			w := httptest.NewRecorder()
			require.NoError(t, response.Apply(ctx, w, req))
			require.Equal(t, tt.status, w.Code)
		})
	}
}
