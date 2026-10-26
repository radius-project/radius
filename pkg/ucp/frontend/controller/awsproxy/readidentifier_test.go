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
			require.Equal(t, identifier, aws.ToString(input.Identifier))
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
		{
			name: "China partition", schema: imageReadSchema, resourceType: "AWS::EC2::Image", region: "cn-north-1",
			properties: map[string]any{"ImageId": "ami-123"}, identifier: "arn:aws-cn:ec2:cn-north-1::image/ami-123",
		},
		{
			name: "GovCloud partition", schema: imageReadSchema, resourceType: "AWS::EC2::Image", region: "us-gov-west-1",
			properties: map[string]any{"ImageId": "ami-123"}, identifier: "arn:aws-us-gov:ec2:us-gov-west-1::image/ami-123",
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
			id, response, err := controller.resolveReadIdentifier(t.Context(), tt.properties, tt.schema, tt.resourceType, tt.region)
			require.NoError(t, err)
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
			name: "missing read properties is an error", pages: []cloudcontrol.ListResourcesOutput{{ResourceDescriptions: []types.ResourceDescription{{Identifier: matching.Identifier}}}},
			hydrated:      &cloudcontrol.GetResourceOutput{ResourceDescription: &types.ResourceDescription{Properties: aws.String(`{}`)}},
			errorContains: "missing additional identifiers",
		},
		{name: "list error", pages: []cloudcontrol.ListResourcesOutput{{}}, listErr: errors.New("list denied"), errorContains: "list denied"},
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
						return &page, tt.listErr
					})
			}
			if tt.hydrated != nil || tt.hydrationErr != nil {
				options.AWSCloudControlClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, input *cloudcontrol.GetResourceInput, _ ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceOutput, error) {
						require.Equal(t, arn, aws.ToString(input.Identifier))
						return tt.hydrated, tt.hydrationErr
					})
			}
			controller := &GetAWSResourceWithPost{awsClients: ucpaws.Clients{CloudControl: options.AWSCloudControlClient}}
			id, response, err := controller.resolveReadIdentifier(t.Context(), map[string]any{"ModelId": "amazon.titan-embed-text-v2:0"}, modelReadSchema, "AWS::Bedrock::FoundationModel", "us-west-2")
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
	id, response, err := controller.resolveReadIdentifier(t.Context(), map[string]any{"WorkGroup": "workgroup", "SessionId": "session"}, sessionReadSchema, "AWS::Athena::Session", "us-west-2")
	require.NoError(t, err)
	require.Nil(t, response)
	require.Equal(t, "matching", id)
}

func Test_ResolveReadIdentifier_AlternativeIdentifierSets(t *testing.T) {
	t.Parallel()
	options := setupTest(t)
	options.AWSCloudControlClient.EXPECT().ListResources(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&cloudcontrol.ListResourcesOutput{ResourceDescriptions: []types.ResourceDescription{
			{Identifier: aws.String("matching"), Properties: aws.String(`{"Name":"name"}`)},
		}}, nil)
	controller := &GetAWSResourceWithPost{awsClients: ucpaws.Clients{CloudControl: options.AWSCloudControlClient}}
	schema := `{"primaryIdentifier":["/properties/Arn"],"additionalIdentifiers":[["/properties/Parent","/properties/Child"],["/properties/Name"]]}`
	id, response, err := controller.resolveReadIdentifier(t.Context(), map[string]any{"Name": "name", "Parent": "parent"}, schema, "AWS::Example::Resource", "us-west-2")
	require.NoError(t, err)
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
