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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	test "github.com/radius-project/radius/test/ucp"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
)

// Test_AWS_RedeployReorderedTags redeploys a log group with its tags in the reverse order. Tags are an unordered array
// in the log group schema, so UCP must treat the redeploy as unchanged and return 200 without calling AWS.
func Test_AWS_RedeployReorderedTags(t *testing.T) {
	ctx := t.Context()

	myTest := test.NewUCPTest(t, "Test_AWS_RedeployReorderedTags", func(t *testing.T, url string, roundTripper http.RoundTripper) {
		logGroupName := generateLogGroupName(t)
		setupTestAWSResource(t, ctx, logGroupName)
		resourceID, err := validation.GetResourceIdentifier(ctx, logGroupResourceType, logGroupName)
		require.NoError(t, err)
		putURL := fmt.Sprintf("%s%s?api-version=%s", url, resourceID, v20231001preview.Version)

		tags := []any{
			map[string]any{"Key": "first", "Value": "1"},
			map[string]any{"Key": "second", "Value": "2"},
			map[string]any{"Key": "third", "Value": "3"},
		}
		put := func(tags []any) *http.Response {
			body, err := json.Marshal(map[string]any{
				"properties": map[string]any{
					"LogGroupName":    logGroupName,
					"RetentionInDays": 7,
					"Tags":            tags,
				},
			})
			require.NoError(t, err)
			return sendAWSRequest(t, roundTripper, http.MethodPut, putURL, body)
		}

		// The first deploy adds the tags and retention, so UCP updates the log group.
		response := put(tags)
		requireResponseStatus(t, response, http.StatusCreated)
		require.NoError(t, response.Body.Close())
		waitForUCPOperation(t, roundTripper, response.Header.Get("Azure-Asyncoperation"))

		// The second deploy only reverses the tags.
		reversed := []any{tags[2], tags[1], tags[0]}
		response = put(reversed)
		defer response.Body.Close()
		requireResponseStatus(t, response, http.StatusOK)
	})

	myTest.RequiredFeatures = []test.RequiredFeature{test.FeatureAWS}
	myTest.Test(t)
}

func waitForUCPOperation(t *testing.T, roundTripper http.RoundTripper, statusURL string) {
	t.Helper()
	require.NotEmpty(t, statusURL)
	for range 100 {
		request, err := http.NewRequest(http.MethodGet, statusURL, nil)
		require.NoError(t, err)
		response, err := roundTripper.RoundTrip(request)
		require.NoError(t, err)
		payload, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		require.Equal(t, http.StatusOK, response.StatusCode, string(payload))

		body := map[string]any{}
		require.NoError(t, json.Unmarshal(payload, &body))
		switch body["status"] {
		case "Succeeded":
			return
		case "Failed", "Canceled":
			require.Failf(t, "operation did not succeed", "%s", string(payload))
		}
		time.Sleep(1 * time.Second)
	}
	require.Fail(t, "operation did not finish in time")
}
