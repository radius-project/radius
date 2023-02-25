// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package aws

// Tests that test with Mock RP functionality and UCP Server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/project-radius/radius/pkg/to"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const ZeroAWSRequestToken = "00000000-0000-0000-0000-000000000000"

func Test_UpdateAWSResource(t *testing.T) {
	ucp, ucpClient, cloudcontrolClient, cloudFormationClient := initializeTest(t)

	getResponseBody := map[string]any{
		"RetentionPeriodHours": 178,
		"ShardCount":           3,
	}
	getResponseBodyBytes, err := json.Marshal(getResponseBody)
	require.NoError(t, err)

	resourceType := "AWS::Kinesis::Stream"
	typeSchema := map[string]any{
		"readOnlyProperties": []any{
			"/properties/Arn",
		},
		"createOnlyProperties": []any{
			"/properties/Name",
		},
	}
	serialized, err := json.Marshal(typeSchema)
	require.NoError(t, err)
	output := cloudformation.DescribeTypeOutput{
		TypeName: aws.String(resourceType),
		Schema:   aws.String(string(serialized)),
	}

	cloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any()).Return(&output, nil)

	cloudcontrolClient.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params *cloudcontrol.GetResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceOutput, error) {
		output := cloudcontrol.GetResourceOutput{
			ResourceDescription: &types.ResourceDescription{
				Properties: to.Ptr(string(getResponseBodyBytes)),
			},
		}
		return &output, nil
	})

	cloudcontrolClient.EXPECT().UpdateResource(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params *cloudcontrol.UpdateResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.UpdateResourceOutput, error) {
		output := cloudcontrol.UpdateResourceOutput{
			ProgressEvent: &types.ProgressEvent{
				OperationStatus: types.OperationStatusSuccess,
				RequestToken:    to.Ptr(testAWSRequestToken),
			},
		}
		return &output, nil
	})

	requestBody := map[string]any{
		"properties": map[string]any{
			"RetentionPeriodHours": 180,
			"ShardCount":           4,
		},
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)
	updateRequest, err := http.NewRequest(http.MethodPut, ucp.URL+basePath+testProxyRequestAWSPath, bytes.NewBuffer(body))
	require.NoError(t, err)
	updateResponse, err := ucpClient.httpClient.Do(updateRequest)
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, updateResponse.StatusCode)
}
