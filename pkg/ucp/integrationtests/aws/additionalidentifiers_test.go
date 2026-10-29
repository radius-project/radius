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

package aws

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_GetAWSResourceWithPost_ImageID(t *testing.T) {
	ucp, cloudcontrolClient, cloudformationClient := initializeAWSTest(t)
	const collection = "/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/AWS.EC2/Image"
	const imageID = "ami-0829b74b2a8c619b7"
	const arn = "arn:aws:ec2:us-west-2::image/" + imageID
	cloudformationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *cloudformation.DescribeTypeInput, opts ...func(*cloudformation.Options)) (*cloudformation.DescribeTypeOutput, error) {
			require.Equal(t, "AWS::EC2::Image", aws.ToString(input.TypeName))
			options := cloudformation.Options{}
			for _, opt := range opts {
				opt(&options)
			}
			require.Equal(t, "us-west-2", options.Region)
			return &cloudformation.DescribeTypeOutput{Schema: aws.String(`{
				"primaryIdentifier":["/properties/Arn"],
				"additionalIdentifiers":[["/properties/ImageId"]]
			}`)}, nil
		})
	cloudcontrolClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *cloudcontrol.GetResourceInput, opts ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceOutput, error) {
			require.Equal(t, "AWS::EC2::Image", aws.ToString(input.TypeName))
			require.JSONEq(t, `{"ImageId":"`+imageID+`"}`, aws.ToString(input.Identifier))
			options := cloudcontrol.Options{}
			for _, opt := range opts {
				opt(&options)
			}
			require.Equal(t, "us-west-2", options.Region)
			return &cloudcontrol.GetResourceOutput{ResourceDescription: &types.ResourceDescription{
				Identifier: aws.String(arn),
				Properties: aws.String(`{"ImageId":"` + imageID + `","Arn":"` + arn + `"}`),
			}}, nil
		})

	req, err := rpctest.NewHTTPRequestWithContent(t.Context(), http.MethodPost, ucp.BaseURL()+collection+"/:get",
		[]byte(`{"properties":{"ImageId":"`+imageID+`"}}`))
	require.NoError(t, err)
	response, err := ucp.Client().Do(req)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	var body map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	require.Equal(t, arn, body["name"])
	require.Equal(t, collection+"/"+arn, body["id"])
	require.Equal(t, "AWS.EC2/Image", body["type"])
	require.Equal(t, map[string]any{"ImageId": imageID, "Arn": arn}, body["properties"])
}

func Test_GetAWSResourceWithPost_AdditionalIdentifierList(t *testing.T) {
	const collection = "/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/AWS.Bedrock/FoundationModel"
	const modelID = "amazon.titan-embed-text-v2:0"
	const arn = "arn:aws:bedrock:us-west-2::foundation-model/" + modelID
	matching := types.ResourceDescription{Identifier: aws.String(arn), Properties: aws.String(`{"ModelId":"` + modelID + `"}`)}
	other := types.ResourceDescription{Identifier: aws.String("other-arn"), Properties: aws.String(`{"ModelId":"other-model"}`)}

	for _, tt := range []struct {
		name   string
		pages  []cloudcontrol.ListResourcesOutput
		status int
	}{
		{
			name: "match on later page", status: http.StatusOK,
			pages: []cloudcontrol.ListResourcesOutput{
				{ResourceDescriptions: []types.ResourceDescription{other}, NextToken: aws.String("page2")},
				{ResourceDescriptions: []types.ResourceDescription{matching}},
			},
		},
		{
			name: "no match", status: http.StatusNotFound,
			pages: []cloudcontrol.ListResourcesOutput{
				{NextToken: aws.String("page2")},
				{ResourceDescriptions: []types.ResourceDescription{other}},
			},
		},
		{
			name: "two matches", status: http.StatusConflict,
			pages: []cloudcontrol.ListResourcesOutput{
				{ResourceDescriptions: []types.ResourceDescription{matching}, NextToken: aws.String("page2")},
				{ResourceDescriptions: []types.ResourceDescription{{Identifier: aws.String("second-arn"), Properties: matching.Properties}}},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ucp, cloudcontrolClient, cloudformationClient := initializeAWSTest(t)
			cloudformationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&cloudformation.DescribeTypeOutput{Schema: aws.String(`{
					"primaryIdentifier":["/properties/ModelArn"],
					"additionalIdentifiers":[["/properties/ModelId"]]
				}`)}, nil)
			calls := []any{cloudcontrolClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, input *cloudcontrol.GetResourceInput, _ ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceOutput, error) {
					require.JSONEq(t, `{"ModelId":"`+modelID+`"}`, aws.ToString(input.Identifier))
					return nil, &types.UnsupportedActionException{}
				}).Call}
			for i := range tt.pages {
				calls = append(calls, cloudcontrolClient.EXPECT().ListResources(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, input *cloudcontrol.ListResourcesInput, opts ...func(*cloudcontrol.Options)) (*cloudcontrol.ListResourcesOutput, error) {
						require.Equal(t, "AWS::Bedrock::FoundationModel", aws.ToString(input.TypeName))
						require.JSONEq(t, `{"ModelId":"`+modelID+`"}`, aws.ToString(input.ResourceModel))
						if i == 0 {
							require.Nil(t, input.NextToken)
						} else {
							require.Equal(t, "page2", aws.ToString(input.NextToken))
						}
						options := cloudcontrol.Options{}
						for _, opt := range opts {
							opt(&options)
						}
						require.Equal(t, "us-west-2", options.Region)
						return &tt.pages[i], nil
					}).Call)
			}
			if tt.status == http.StatusOK {
				calls = append(calls, cloudcontrolClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, input *cloudcontrol.GetResourceInput, _ ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceOutput, error) {
						require.Equal(t, "AWS::Bedrock::FoundationModel", aws.ToString(input.TypeName))
						require.Equal(t, arn, aws.ToString(input.Identifier))
						return &cloudcontrol.GetResourceOutput{ResourceDescription: &matching}, nil
					}).Call)
			}
			gomock.InOrder(calls...)
			req, err := rpctest.NewHTTPRequestWithContent(t.Context(), http.MethodPost, ucp.BaseURL()+collection+"/:get",
				[]byte(`{"properties":{"ModelId":"`+modelID+`"}}`))
			require.NoError(t, err)
			response, err := ucp.Client().Do(req)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, tt.status, response.StatusCode)
			var body map[string]any
			require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
			if tt.status == http.StatusOK {
				require.Equal(t, arn, body["name"])
				require.Equal(t, collection+"/"+arn, body["id"])
				require.Equal(t, "AWS.Bedrock/FoundationModel", body["type"])
				require.Equal(t, map[string]any{"ModelId": modelID}, body["properties"])
			} else {
				require.NotEmpty(t, body["error"])
				require.NotContains(t, body, "properties")
			}
		})
	}
}
