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
	logGroupResourceType    = "AWS.Logs/LogGroup"
	awsLogGroupResourceType = "AWS::Logs::LogGroup"
)

func Test_AWS_DeleteResource(t *testing.T) {
	ctx := context.Background()

	myTest := test.NewUCPTest(t, "Test_AWS_DeleteResource", func(t *testing.T, url string, roundTripper http.RoundTripper) {
		logGroupName := generateLogGroupName()
		t.Logf("Setting up test AWS resource with log group name: %s", logGroupName)
		setupTestAWSResource(t, ctx, logGroupName)
		resourceID, err := validation.GetResourceIdentifier(ctx, logGroupResourceType, logGroupName)
		require.NoError(t, err)
		t.Logf("Retrieved resource ID: %s", resourceID)

		// Construct resource collection url
		resourceIDParts := strings.Split(resourceID, "/")
		resourceIDParts = resourceIDParts[:len(resourceIDParts)-1]
		resourceID = strings.Join(resourceIDParts, "/")
		deleteURL := fmt.Sprintf("%s%s/:delete?api-version=%s", url, resourceID, v20231001preview.Version)
		t.Logf("DELETE operation URL: %s", deleteURL)
		
		deleteRequestBody := map[string]any{
			"properties": map[string]any{
				"LogGroupName": logGroupName,
			},
		}
		deleteBody, err := json.Marshal(deleteRequestBody)
		require.NoError(t, err)
		t.Logf("DELETE request body: %s", string(deleteBody))


		// Issue the Delete Request
		deleteRequest, err := http.NewRequest(http.MethodPost, deleteURL, bytes.NewBuffer(deleteBody))
		require.NoError(t, err)
		t.Logf("Sending DELETE request to: %s", deleteRequest.URL.String())
		deleteResponse, err := roundTripper.RoundTrip(deleteRequest)
		require.NoError(t, err)
		require.Equal(t, http.StatusAccepted, deleteResponse.StatusCode)
		t.Logf("DELETE request completed with status: %d", deleteResponse.StatusCode)

		// Get the operation status url from the Azure-Asyncoperation header
		deleteResponseCompletionUrl := deleteResponse.Header["Azure-Asyncoperation"][0]
		t.Logf("DELETE operation completion URL: %s", deleteResponseCompletionUrl)
		getRequest, err := http.NewRequest(http.MethodGet, deleteResponseCompletionUrl, nil)
		require.NoError(t, err)
		maxRetries := 100
		deleteSucceeded := false
		for i := 0; i < maxRetries; i++ {
			t.Logf("Polling DELETE operation status (attempt %d/%d): %s", i+1, maxRetries, deleteResponseCompletionUrl)
			getResponse, err := roundTripper.RoundTrip(getRequest)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, getResponse.StatusCode)

			// Read the request status from the body
			defer getResponse.Body.Close()
			payload, err := io.ReadAll(getResponse.Body)
			require.NoError(t, err)
			body := map[string]any{}
			err = json.Unmarshal(payload, &body)
			require.NoError(t, err)
			t.Logf("DELETE operation status response: %s", string(payload))
			if body["status"] == "Succeeded" {
				deleteSucceeded = true
				t.Logf("DELETE operation succeeded after %d attempts", i+1)
				break
			}
			// Give it more time
			time.Sleep(1 * time.Second)
		}
		require.True(t, deleteSucceeded)
	})

	myTest.RequiredFeatures = []test.RequiredFeature{test.FeatureAWS}
	myTest.Test(t)
}

func Test_AWS_ListResources(t *testing.T) {
	ctx := context.Background()

	myTest := test.NewUCPTest(t, "Test_AWS_ListResources", func(t *testing.T, url string, roundTripper http.RoundTripper) {
		var logGroupName = generateLogGroupName()
		t.Logf("Setting up test AWS resource with log group name: %s", logGroupName)
		setupTestAWSResource(t, ctx, logGroupName)
		resourceID, err := validation.GetResourceIdentifier(ctx, logGroupResourceType, logGroupName)
		require.NoError(t, err)
		t.Logf("Retrieved resource ID: %s", resourceID)

		// Construct resource collection url
		resourceIDParts := strings.Split(resourceID, "/")
		resourceIDParts = resourceIDParts[:len(resourceIDParts)-1]
		resourceID = strings.Join(resourceIDParts, "/")
		listURL := fmt.Sprintf("%s%s?api-version=%s", url, resourceID, v20231001preview.Version)
		t.Logf("LIST operation URL: %s", listURL)

		// Issue the List Request
		listRequest, err := http.NewRequest(http.MethodGet, listURL, nil)
		require.NoError(t, err)
		t.Logf("Sending LIST request to: %s", listRequest.URL.String())
		listResponse, err := roundTripper.RoundTrip(listRequest)
		require.NoError(t, err)

		require.Equal(t, http.StatusOK, listResponse.StatusCode)
		t.Logf("LIST request completed with status: %d", listResponse.StatusCode)

		defer listResponse.Body.Close()
		payload, err := io.ReadAll(listResponse.Body)
		require.NoError(t, err)
		t.Logf("LIST response body: %s", string(payload))
		body := map[string][]any{}
		err = json.Unmarshal(payload, &body)
		require.NoError(t, err)

		// Verify payload has at least one resource
		require.Len(t, body, 1)
		require.GreaterOrEqual(t, len(body["value"]), 1)
		t.Logf("LIST operation returned %d resources", len(body["value"]))
	})

	myTest.RequiredFeatures = []test.RequiredFeature{test.FeatureAWS}
	myTest.Test(t)
}

func setupTestAWSResource(t *testing.T, ctx context.Context, resourceName string) {
	// Test setup - Create AWS resource using AWS APIs
	t.Logf("Starting AWS resource setup for: %s", resourceName)
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	require.NoError(t, err)
	var awsClient aws.AWSCloudControlClient = cloudcontrol.NewFromConfig(cfg)
	desiredState := map[string]any{
		"LogGroupName": resourceName,
	}
	desiredStateBytes, err := json.Marshal(desiredState)
	require.NoError(t, err)
	t.Logf("CREATE AWS resource desired state: %s", string(desiredStateBytes))

	cloudControlOpts := []func(*cloudcontrol.Options){awsproxy.CloudControlRegionOption("us-west-2")}

	t.Logf("Creating AWS resource via AWS SDK: %s (type: %s)", resourceName, awsLogGroupResourceType)
	response, err := awsClient.CreateResource(ctx, &cloudcontrol.CreateResourceInput{
		TypeName:     &awsLogGroupResourceType,
		DesiredState: awsgo.String(string(desiredStateBytes)),
	}, cloudControlOpts...)
	require.NoError(t, err)
	t.Logf("AWS CREATE resource request submitted, request token: %s", *response.ProgressEvent.RequestToken)
	waitForSuccess(t, ctx, awsClient, response.ProgressEvent.RequestToken)
	t.Logf("AWS resource creation completed successfully: %s", resourceName)

	t.Cleanup(func() {
		t.Logf("Starting cleanup for AWS resource: %s", resourceName)
		// Check if resource exists before issuing a delete because the AWS SDK async delete operation
		// seems to fail if the resource does not exist
		_, err := awsClient.GetResource(ctx, &cloudcontrol.GetResourceInput{
			Identifier: &resourceName,
			TypeName:   &awsLogGroupResourceType,
		}, cloudControlOpts...)
		if aws.IsAWSResourceNotFoundError(err) {
			t.Logf("AWS resource not found during cleanup, skipping delete: %s", resourceName)
			return
		}
		// Just in case delete fails
		t.Logf("Deleting AWS resource via AWS SDK: %s", resourceName)
		deleteOutput, err := awsClient.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{
			Identifier: &resourceName,
			TypeName:   &awsLogGroupResourceType,
		}, cloudControlOpts...)
		require.NoError(t, err)
		t.Logf("AWS DELETE resource request submitted, request token: %s", *deleteOutput.ProgressEvent.RequestToken)

		// Ignoring status of delete since AWS command fails if the resource does not already exist
		waitForSuccess(t, ctx, awsClient, deleteOutput.ProgressEvent.RequestToken)
		t.Logf("AWS resource cleanup completed: %s", resourceName)
	})
	// End of test setup
}

func waitForSuccess(t *testing.T, ctx context.Context, awsClient aws.AWSCloudControlClient, requestToken *string) {
	// Wait till the create is complete
	t.Logf("Waiting for AWS operation to complete, request token: %s", *requestToken)
	maxWaitTime := 300 * time.Second
	waiter := cloudcontrol.NewResourceRequestSuccessWaiter(awsClient)
	err := waiter.Wait(ctx, &cloudcontrol.GetResourceRequestStatusInput{
		RequestToken: requestToken,
	}, maxWaitTime)
	require.NoError(t, err)
	t.Logf("AWS operation completed successfully, request token: %s", *requestToken)
}

func generateLogGroupName() string {
	return "ucpfunctionaltest-" + uuid.NewString()
}
