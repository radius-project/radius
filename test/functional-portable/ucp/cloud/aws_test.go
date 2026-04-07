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
	"sort"
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
	ctx := t.Context()

	myTest := test.NewUCPTest(t, "Test_AWS_DeleteResource", func(t *testing.T, url string, roundTripper http.RoundTripper) {
		logGroupName := generateLogGroupName(t)
		setupTestAWSResource(t, ctx, logGroupName)
		resourceID, err := validation.GetResourceIdentifier(ctx, logGroupResourceType, logGroupName)
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
		requireResponseStatus(t, deleteResponse, http.StatusAccepted)
		defer deleteResponse.Body.Close()

		// Get the operation status url from the Azure-Asyncoperation header
		deleteResponseCompletionUrl := deleteResponse.Header["Azure-Asyncoperation"][0]
		getRequest, err := http.NewRequest(http.MethodGet, deleteResponseCompletionUrl, nil)
		require.NoError(t, err)
		maxRetries := 100
		deleteSucceeded := false
		for range maxRetries {
			getResponse, err := roundTripper.RoundTrip(getRequest)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, getResponse.StatusCode)

			// Read the request status from the body
			payload, err := io.ReadAll(getResponse.Body)
			require.NoError(t, err)
			require.NoError(t, getResponse.Body.Close())
			body := map[string]any{}
			err = json.Unmarshal(payload, &body)
			require.NoError(t, err)
			if body["status"] == "Succeeded" {
				deleteSucceeded = true
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
	ctx := t.Context()

	myTest := test.NewUCPTest(t, "Test_AWS_ListResources", func(t *testing.T, url string, roundTripper http.RoundTripper) {
		var logGroupName = generateLogGroupName(t)
		setupTestAWSResource(t, ctx, logGroupName)
		resourceID, err := validation.GetResourceIdentifier(ctx, logGroupResourceType, logGroupName)
		require.NoError(t, err)

		// Construct resource collection url
		resourceIDParts := strings.Split(resourceID, "/")
		resourceIDParts = resourceIDParts[:len(resourceIDParts)-1]
		resourceID = strings.Join(resourceIDParts, "/")
		listURL := fmt.Sprintf("%s%s?api-version=%s", url, resourceID, v20231001preview.Version)

		// Issue the List Request
		listRequest, err := http.NewRequest(http.MethodGet, listURL, nil)
		require.NoError(t, err)
		listResponse, err := roundTripper.RoundTrip(listRequest)
		require.NoError(t, err)

		requireResponseStatus(t, listResponse, http.StatusOK)

		defer listResponse.Body.Close()
		payload, err := io.ReadAll(listResponse.Body)
		require.NoError(t, err)
		body := map[string][]any{}
		err = json.Unmarshal(payload, &body)
		require.NoError(t, err)

		// Verify payload has at least one resource
		require.Len(t, body, 1)
		require.GreaterOrEqual(t, len(body["value"]), 1)
	})

	myTest.RequiredFeatures = []test.RequiredFeature{test.FeatureAWS}
	myTest.Test(t)
}

func setupTestAWSResource(t *testing.T, ctx context.Context, resourceName string) {
	// Test setup - Create AWS resource using AWS APIs
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	require.NoError(t, err)
	var awsClient aws.AWSCloudControlClient = cloudcontrol.NewFromConfig(cfg)
	desiredState := map[string]any{
		"LogGroupName": resourceName,
	}
	desiredStateBytes, err := json.Marshal(desiredState)
	require.NoError(t, err)

	cloudControlOpts := []func(*cloudcontrol.Options){awsproxy.CloudControlRegionOption("us-west-2")}

	response, err := awsClient.CreateResource(ctx, &cloudcontrol.CreateResourceInput{
		TypeName:     &awsLogGroupResourceType,
		DesiredState: awsgo.String(string(desiredStateBytes)),
	}, cloudControlOpts...)
	require.NoError(t, err)
	waitForSuccess(t, ctx, awsClient, response.ProgressEvent.RequestToken)

	t.Cleanup(func() {
		// Use a fresh context because t.Context() is cancelled before cleanup runs.
		cleanupCtx := context.Background()

		// Check if resource exists before issuing a delete because the AWS SDK async delete operation
		// seems to fail if the resource does not exist
		_, err := awsClient.GetResource(cleanupCtx, &cloudcontrol.GetResourceInput{
			Identifier: &resourceName,
			TypeName:   &awsLogGroupResourceType,
		}, cloudControlOpts...)
		if aws.IsAWSResourceNotFoundError(err) {
			return
		}
		// Just in case delete fails
		deleteOutput, err := awsClient.DeleteResource(cleanupCtx, &cloudcontrol.DeleteResourceInput{
			Identifier: &resourceName,
			TypeName:   &awsLogGroupResourceType,
		}, cloudControlOpts...)
		require.NoError(t, err)

		// Ignoring status of delete since AWS command fails if the resource does not already exist
		waitForSuccess(t, cleanupCtx, awsClient, deleteOutput.ProgressEvent.RequestToken)
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

func generateLogGroupName(t *testing.T) string {
	t.Helper()
	return "ucpfunctionaltest-" + uuid.NewString()
}

func requireResponseStatus(t *testing.T, response *http.Response, expectedStatus int) {
	t.Helper()
	require.NotNil(t, response)

	if response.StatusCode == expectedStatus {
		return
	}

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())

	response.Body = io.NopCloser(bytes.NewReader(body))

	headerKeys := make([]string, 0, len(response.Header))
	for key := range response.Header {
		headerKeys = append(headerKeys, key)
	}
	sort.Strings(headerKeys)

	headers := make([]string, 0, len(headerKeys))
	for _, key := range headerKeys {
		headers = append(headers, fmt.Sprintf("%s=%s", key, strings.Join(response.Header.Values(key), ",")))
	}

	require.Failf(t, "unexpected response status",
		"expected status %d, got %d\nheaders: %s\nbody: %s",
		expectedStatus,
		response.StatusCode,
		strings.Join(headers, "; "),
		string(body),
	)
}
