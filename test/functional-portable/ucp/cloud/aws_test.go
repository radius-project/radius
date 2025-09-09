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

package ucp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	awsgo "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/frontend/controller/awsproxy"
	test "github.com/radius-project/radius/test/ucp"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
)

var (
	logsLogGroupResourceType    = "AWS.Logs/LogGroup"
	awsLogsLogGroupResourceType = "AWS::Logs::LogGroup"
)

func Test_AWS_DeleteResource_LogGroup(t *testing.T) {
	ctx := context.Background()

	myTest := test.NewUCPTest(t, "Test_AWS_DeleteResource_LogGroup", func(t *testing.T, url string, roundTripper http.RoundTripper) {
		logGroupName := generateLogGroupName()
		setupTestAWSResource(t, ctx, logGroupName)
		resourceID, err := validation.GetResourceIdentifier(ctx, logsLogGroupResourceType, logGroupName)
		require.NoError(t, err)

		// Construct resource collection url
		resourceIDParts := strings.Split(resourceID, "/")
		resourceIDParts = resourceIDParts[:len(resourceIDParts)-1]
		resourceID = strings.Join(resourceIDParts, "/")
		deleteURL := fmt.Sprintf("%s%s/:delete?api-version=%s", url, resourceID, v20231001preview.Version)
		deleteRequestBody := map[string]any{
			"properties": map[string]any{
				"LogGroupName": logGroupName,
			},
		}
		deleteBody, err := json.Marshal(deleteRequestBody)
		require.NoError(t, err)

		// Issue the Delete Request
		deleteRequest, err := http.NewRequest(http.MethodPost, deleteURL, bytes.NewBuffer(deleteBody))
		require.NoError(t, err)
		deleteResponse, err := roundTripper.RoundTrip(deleteRequest)
		require.NoError(t, err)
		require.Equal(t, http.StatusAccepted, deleteResponse.StatusCode)

		// Get the operation status url from the Azure-Asyncoperation header
		deleteResponseCompletionUrl := deleteResponse.Header["Azure-Asyncoperation"][0]
		getRequest, err := http.NewRequest(http.MethodGet, deleteResponseCompletionUrl, nil)
		require.NoError(t, err)
		maxRetries := 100
		for i := 0; i < maxRetries; i++ {
			getResponse, err := roundTripper.RoundTrip(getRequest)
			require.NoError(t, err)
			body := map[string]any{}
			bodyBytes, err := io.ReadAll(getResponse.Body)
			require.NoError(t, err)
			err = json.Unmarshal(bodyBytes, &body)
			require.NoError(t, err)
			if body["status"].(string) == "Succeeded" {
				break
			}
			time.Sleep(1 * time.Second)
		}

		// Validate that the resource was deleted
		cfg, err := awsconfig.LoadDefaultConfig(ctx)
		require.NoError(t, err)
		var awsClient aws.AWSCloudControlClient = cloudcontrol.NewFromConfig(cfg)
		cloudControlOpts := []func(*cloudcontrol.Options){awsproxy.CloudControlRegionOption("us-west-2")}

		_, err = awsClient.GetResource(ctx, &cloudcontrol.GetResourceInput{
			Identifier: &logGroupName,
			TypeName:   &awsLogsLogGroupResourceType,
		}, cloudControlOpts...)
		require.True(t, aws.IsAWSResourceNotFoundError(err))
	})

	myTest.RequiredFeatures = []test.RequiredFeature{test.FeatureAWS}
	myTest.Test(t)
}

func setupTestAWSResource(t *testing.T, ctx context.Context, logGroupName string) {
	// Test setup - Create AWS Resource using AWS APIs
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	require.NoError(t, err)
	var awsClient aws.AWSCloudControlClient = cloudcontrol.NewFromConfig(cfg)
	desiredState := map[string]any{
		"LogGroupName":    logGroupName,
		"RetentionInDays": float64(7),
		"Tags": []map[string]string{
			{
				"Key":   "testKey",
				"Value": "testValue",
			},
		},
	}
	desiredStateBytes, err := json.Marshal(desiredState)
	require.NoError(t, err)

	cloudControlOpts := []func(*cloudcontrol.Options){awsproxy.CloudControlRegionOption("us-west-2")}

	response, err := awsClient.CreateResource(ctx, &cloudcontrol.CreateResourceInput{
		TypeName:     &awsLogsLogGroupResourceType,
		DesiredState: awsgo.String(string(desiredStateBytes)),
	}, cloudControlOpts...)
	require.NoError(t, err)
	waitForSuccess(t, ctx, awsClient, response.ProgressEvent.RequestToken)

	t.Cleanup(func() {
		// Check if resource exists before issuing a delete because the AWS SDK async delete operation
		// seems to fail if the resource does not exist
		_, err := awsClient.GetResource(ctx, &cloudcontrol.GetResourceInput{
			Identifier: &logGroupName,
			TypeName:   &awsLogsLogGroupResourceType,
		}, cloudControlOpts...)
		if aws.IsAWSResourceNotFoundError(err) {
			return
		}
		// Just in case delete fails
		deleteOutput, err := awsClient.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{
			Identifier: &logGroupName,
			TypeName:   &awsLogsLogGroupResourceType,
		}, cloudControlOpts...)
		require.NoError(t, err)

		// Ignoring status of delete since AWS command fails if the resource does not already exist
		waitForSuccess(t, ctx, awsClient, deleteOutput.ProgressEvent.RequestToken)
	})
	// End of test setup
}

func waitForSuccess(t *testing.T, ctx context.Context, awsClient aws.AWSCloudControlClient, requestToken *string) {
	// Wait till the create is complete
	maxWaitTime := 300 * time.Second
	waiter := cloudcontrol.NewResourceRequestSuccessWaiter(awsClient)
	err := waiter.Wait(ctx, &cloudcontrol.GetResourceRequestStatusInput{
		RequestToken: requestToken,
	}, maxWaitTime)
	require.NoError(t, err)
}

func generateLogGroupName() string {
	return "ucpfunctionaltest-" + uuid.NewString()
}
