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
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/project-radius/radius/pkg/armrpc/rpctest"
	"github.com/project-radius/radius/pkg/to"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_CreateAWSResourceWithPost(t *testing.T) {
	ucp, _, _, cloudcontrolClient, cloudformationClient := initializeAWSTest(t)

	primaryIdentifiers := map[string]any{
		"primaryIdentifier": []any{
			"/properties/Name",
		},
	}
	serialized, err := json.Marshal(primaryIdentifiers)
	require.NoError(t, err)
	output := cloudformation.DescribeTypeOutput{
		TypeName: aws.String("AWS::Kinesis::Stream"),
		Schema:   to.Ptr(string(serialized)),
	}

	cloudformationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).Return(&output, nil)

	cloudcontrolClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params *cloudcontrol.GetResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceOutput, error) {
		notfound := types.ResourceNotFoundException{
			Message: to.Ptr("Resource not found"),
		}
		return nil, &notfound
	})

	cloudcontrolClient.EXPECT().CreateResource(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params *cloudcontrol.CreateResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.CreateResourceOutput, error) {
		output := cloudcontrol.CreateResourceOutput{
			ProgressEvent: &types.ProgressEvent{
				OperationStatus: types.OperationStatusSuccess,
				RequestToken:    to.Ptr(testAWSRequestToken),
				Identifier:      to.Ptr(testAWSResourceName),
			},
		}
		return &output, nil
	})

	requestBody := map[string]any{
		"properties": map[string]any{
			"Name":                 "testStream",
			"RetentionPeriodHours": 178,
			"ShardCount":           3,
		},
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	createRequest, err := rpctest.NewHTTPRequestWithContent(context.Background(), http.MethodPost, ucp.BaseURL+testProxyRequestAWSCollectionPath+"/:put", body)
	require.NoError(t, err, "creating request failed")

	ctx := rpctest.NewARMRequestContext(createRequest)
	createRequest = createRequest.WithContext(ctx)

	createResponse, err := ucp.Client().Do(createRequest)
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, createResponse.StatusCode)
}
