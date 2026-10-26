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

	"uuid"

	awsgo "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
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

func Test_AWS_GetResourceWithImageID(t *testing.T) {
	myTest := test.NewUCPTest(t, t.Name(), func(t *testing.T, url string, roundTripper http.RoundTripper) {
		ctx := t.Context()
		cfg, err := awsconfig.LoadDefaultConfig(ctx)
		require.NoError(t, err)
		pages := ec2.NewDescribeImagesPaginator(ec2.NewFromConfig(cfg), &ec2.DescribeImagesInput{
			Owners: []string{"amazon"},
			Filters: []ec2types.Filter{
				{Name: awsgo.String("name"), Values: []string{"al2023-ami-2023.*-x86_64"}},
				{Name: awsgo.String("architecture"), Values: []string{"x86_64"}},
				{Name: awsgo.String("state"), Values: []string{"available"}},
				{Name: awsgo.String("is-public"), Values: []string{"true"}},
			},
		})
		var newest ec2types.Image
		for pages.HasMorePages() {
			page, err := pages.NextPage(ctx)
			require.NoError(t, err)
			for _, image := range page.Images {
				if awsgo.ToString(image.CreationDate) > awsgo.ToString(newest.CreationDate) {
					newest = image
				}
			}
		}
		imageID := awsgo.ToString(newest.ImageId)
		require.NotEmpty(t, imageID, "Amazon Linux 2023 must have a public AMI in %s", cfg.Region)

		body := postGetAWSResource(t, ctx, url, roundTripper, "AWS.EC2/Image", map[string]any{"ImageId": imageID})
		properties, ok := body["properties"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, imageID, properties["ImageId"])
		require.Equal(t, properties["Arn"], body["name"])
		require.NotEmpty(t, properties["Arn"])
		require.Contains(t, body["name"], "::image/"+imageID)
		require.Equal(t, "AWS.EC2/Image", body["type"])
	})
	myTest.RequiredFeatures = []test.RequiredFeature{test.FeatureAWS}
	myTest.Test(t)
}

func Test_AWS_GetResourceWithModelID(t *testing.T) {
	myTest := test.NewUCPTest(t, t.Name(), func(t *testing.T, url string, roundTripper http.RoundTripper) {
		ctx := t.Context()
		cfg, err := awsconfig.LoadDefaultConfig(ctx)
		require.NoError(t, err)
		pages := cloudcontrol.NewListResourcesPaginator(cloudcontrol.NewFromConfig(cfg), &cloudcontrol.ListResourcesInput{
			TypeName: awsgo.String("AWS::Bedrock::FoundationModel"),
		})
		var modelID, modelARN string
		for pages.HasMorePages() && modelID == "" {
			page, err := pages.NextPage(ctx)
			require.NoError(t, err)
			for _, model := range page.ResourceDescriptions {
				if model.Properties == nil {
					continue
				}
				var properties map[string]any
				require.NoError(t, json.Unmarshal([]byte(awsgo.ToString(model.Properties)), &properties))
				if id, ok := properties["ModelId"].(string); ok && id != "" {
					modelID = id
					modelARN = awsgo.ToString(model.Identifier)
					break
				}
			}
		}
		require.NotEmpty(t, modelID, "Bedrock must list a foundation model in %s", cfg.Region)
		require.NotEmpty(t, modelARN)

		body := postGetAWSResource(t, ctx, url, roundTripper, "AWS.Bedrock/FoundationModel", map[string]any{"ModelId": modelID})
		properties, ok := body["properties"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, modelID, properties["ModelId"])
		require.Equal(t, modelARN, body["name"])
		require.Equal(t, modelARN, properties["ModelArn"])
		require.Equal(t, "AWS.Bedrock/FoundationModel", body["type"])
	})
	myTest.RequiredFeatures = []test.RequiredFeature{test.FeatureAWS}
	myTest.Test(t)
}

func postGetAWSResource(t *testing.T, ctx context.Context, url string, roundTripper http.RoundTripper, resourceType string, properties map[string]any) map[string]any {
	t.Helper()
	collection, err := validation.GetResourceIdentifier(ctx, resourceType, "")
	require.NoError(t, err)
	payload, err := json.Marshal(map[string]any{"properties": properties})
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s%s:get?api-version=%s", url, collection, v20231001preview.Version), bytes.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	response, err := roundTripper.RoundTrip(req)
	require.NoError(t, err)
	defer response.Body.Close()
	requireResponseStatus(t, response, http.StatusOK)
	var body map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	require.Equal(t, strings.TrimSuffix(collection, "/")+"/"+fmt.Sprint(body["name"]), body["id"])
	return body
}

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
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute) //nolint:usetesting
		defer cancel()

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
	return "ucpfunctionaltest-" + uuid.New().String()
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
