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

package samples

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/kubernetes"
	"github.com/radius-project/radius/test/functional"
	"github.com/radius-project/radius/test/functional/shared"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	remotePort   = 3000
	retries      = 3
	retryTimeout = 1 * time.Minute
	retryBackoff = 1 * time.Second
)

var samplesRepoAbsPath, samplesRepoEnvVarSet = os.LookupEnv("RADIUS_SAMPLES_REPO_ROOT")

// Test process must run with RADIUS_SAMPLES_REPO_ROOT env var set to samples repo absolute path
// You can set the variables used by vscode codelens (e.g. 'debug test', 'run test') using 'go.testEnvVars' in vscode settings.json
// Ex: export PROJECT_RADIUS_SAMPLES_REPO_ABS_PATH=/home/uname/src/samples
func Test_FirstApplicationSample(t *testing.T) {
	// TODO: Remove the following statement
	// LJ: Skipping this test to test pipeline for this PR: https://github.com/radius-project/radius/pull/6130
	t.Skipf("Temporary: Skip samples test execution, samples repo still contains Applications.Links resources, which is deprecated in this PR")

	if !samplesRepoEnvVarSet {
		t.Skipf("Skip samples test execution, to enable you must set env var PROJECT_RADIUS_SAMPLES_REPO_ABS_PATH to the absolute path of the radius-project/samples repository")
	}

	cwd, err := os.Getwd()
	require.NoError(t, err)
	relPathSamplesRepo, err := filepath.Rel(cwd, samplesRepoAbsPath)
	require.NoError(t, err)
	template := filepath.Join(relPathSamplesRepo, "demo/app.bicep")
	appName := "demo"
	appNamespace := "tutorial-demo"

	test := shared.NewRPTest(t, appName, []shared.TestStep{
		{
			Executor:                               step.NewDeployExecutor("testdata/tutorial-environment.bicep", functional.GetBicepRecipeRegistry(), functional.GetBicepRecipeVersion()),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
		},
		{

			Executor: step.NewDeployExecutor(template).WithEnvironment("tutorial").WithApplication(appName),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: appName,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "demo",
						Type: validation.ContainersResource,
					},
					{
						Name: "db",
						Type: validation.RedisCachesResource,
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct shared.RPTest) {
				// Set up pod port-forwarding for the pod
				for i := 1; i <= retries; i++ {
					t.Logf("Setting up portforward (attempt %d/%d)", i, retries)
					selector := fmt.Sprintf("%s=%s", kubernetes.LabelRadiusResource, "demo")
					err := testWithPortForward(t, ctx, ct, appNamespace, selector, remotePort)
					if err != nil {
						t.Logf("Failed to test pod via portforward with error: %s", err)
					} else {
						// Successfully ran tests
						return
					}
				}

				require.Fail(t, fmt.Sprintf("tests failed after %d retries", retries))
			},
			// TODO: validation of k8s resources blocked by https://github.com/radius-project/radius/issues/4689
			K8sOutputResources: []unstructured.Unstructured{},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(appName, "demo"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func testWithPortForward(t *testing.T, ctx context.Context, at shared.RPTest, namespace string, container string, remotePort int) error {
	// stopChan will close the port-forward connection on close
	stopChan := make(chan struct{})

	// portChan will be populated with the assigned port once the port-forward connection is opened on it
	portChan := make(chan int)

	// errorChan will contain any errors created from initializing the port-forwarding session
	errorChan := make(chan error)

	go functional.ExposePod(t, ctx, at.Options.K8sClient, at.Options.K8sConfig, namespace, container, remotePort, stopChan, portChan, errorChan)
	defer close(stopChan)

	select {
	case err := <-errorChan:
		return fmt.Errorf("portforward failed with error: %s", err)
	case localPort := <-portChan:
		baseURL := fmt.Sprintf("http://localhost:%d", localPort)
		t.Logf("Portforward session active at %s", baseURL)
		hostname := "localhost"

		// Test base endpoint, i.e., base URL returns a 200
		_, err := sendGetRequest(t, "hostname", baseURL, "", 200)
		if err != nil {
			return err
		}

		// Test GET /api/todos (list)
		listResponse, err := sendGetRequest(t, hostname, baseURL, "api/todos", 200)
		if err != nil {
			return err
		}

		defer listResponse.Body.Close()
		listResponseBody, err := io.ReadAll(listResponse.Body)
		if err != nil {
			return err
		}

		var actualListResponseBody map[string]any
		err = json.Unmarshal(listResponseBody, &actualListResponseBody)
		if err != nil {
			return err
		}

		expectedListResponseBody := map[string]any{
			"items":   []any{},
			"message": nil,
		}
		require.Equal(t, expectedListResponseBody, actualListResponseBody)

		// Test POST /api/todos (create)
		createRequestBody := map[string]string{
			"title": "My TODO Item",
		}
		createRequestBodyBytes, err := json.Marshal(createRequestBody)
		if err != nil {
			return err
		}

		createResponse, err := sendPostRequest(t, hostname, baseURL, "api/todos", &createRequestBodyBytes, 200)
		if err != nil {
			return err
		}

		defer createResponse.Body.Close()
		createResponseBody, err := io.ReadAll(createResponse.Body)
		if err != nil {
			return err
		}

		var createdItem map[string]any
		err = json.Unmarshal(createResponseBody, &createdItem)
		if err != nil {
			return err
		}

		require.Equal(t, "My TODO Item", createdItem["title"])

		// Set generated Id for use later
		itemId := createdItem["id"]

		// Test GET /api/todos (list)
		listResponse, err = sendGetRequest(t, hostname, baseURL, "api/todos", 200)
		if err != nil {
			return err
		}

		defer listResponse.Body.Close()
		listResponseBody, err = io.ReadAll(listResponse.Body)
		if err != nil {
			return err
		}

		err = json.Unmarshal(listResponseBody, &actualListResponseBody)
		if err != nil {
			return err
		}

		expectedListResponseBody = map[string]any{
			"items": []any{
				createdItem,
			},
			"message": nil,
		}
		require.Equal(t, expectedListResponseBody, actualListResponseBody)

		// Test GET /api/todos/:id (get)
		getResponse, err := sendGetRequest(t, hostname, baseURL, fmt.Sprintf("api/todos/%s", itemId), 200)
		if err != nil {
			return err
		}

		defer getResponse.Body.Close()
		getResponseBody, err := io.ReadAll(getResponse.Body)
		if err != nil {
			return err
		}

		var actualGetResponseBody map[string]any
		err = json.Unmarshal(getResponseBody, &actualGetResponseBody)
		if err != nil {
			return err
		}

		expectedGetResponseBody := createdItem
		require.Equal(t, expectedGetResponseBody, actualGetResponseBody)

		// Test PUT /api/todos/:id (update)
		updateRequestBody := map[string]any{
			"id":    createdItem["id"],
			"_id":   createdItem["_id"],
			"title": createdItem["title"],
			"done":  "true",
		}
		updateRequestBodyBytes, err := json.Marshal(updateRequestBody)
		if err != nil {
			return err
		}

		_, err = sendPutRequest(t, hostname, baseURL, fmt.Sprintf("api/todos/%s", itemId), &updateRequestBodyBytes, 200)
		if err != nil {
			return err
		}

		// Test DELETE /api/todos/:id (delete)
		_, err = sendDeleteRequest(t, hostname, baseURL, fmt.Sprintf("api/todos/%s", itemId), 204)
		if err != nil {
			return err
		}

		// Test GET /api/todos (list)
		listResponse, err = sendGetRequest(t, hostname, baseURL, "api/todos", 200)
		if err != nil {
			return err
		}

		defer listResponse.Body.Close()
		listResponseBody, err = io.ReadAll(listResponse.Body)
		if err != nil {
			return err
		}

		err = json.Unmarshal(listResponseBody, &actualListResponseBody)
		if err != nil {
			return err
		}

		expectedListResponseBody = map[string]any{
			"items":   []any{},
			"message": nil,
		}
		require.Equal(t, expectedListResponseBody, actualListResponseBody)

		// All of the requests were successful
		t.Logf("All requests encountered the correct status code")
		return nil
	}
}

func sendRequest(t *testing.T, req *http.Request, expectedStatusCode int) (*http.Response, error) {
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != expectedStatusCode {
		return nil, fmt.Errorf("expected status code %d, got %d", expectedStatusCode, res.StatusCode)
	}

	return res, nil
}

func sendGetRequest(t *testing.T, hostname, baseURL, path string, expectedStatusCode int) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, getURLPath(baseURL, path), nil)
	if err != nil {
		return nil, err
	}
	req.Host = hostname

	return sendRequest(t, req, expectedStatusCode)
}

func sendPostRequest(t *testing.T, hostname, baseURL, path string, body *[]byte, expectedStatusCode int) (*http.Response, error) {
	if body == nil {
		return nil, fmt.Errorf("body cannot be nil")
	}

	bodyReader := bytes.NewReader(*body)
	req, err := http.NewRequest(http.MethodPost, getURLPath(baseURL, path), bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Host = hostname

	return sendRequest(t, req, expectedStatusCode)
}

func sendPutRequest(t *testing.T, hostname, baseURL, path string, body *[]byte, expectedStatusCode int) (*http.Response, error) {
	if body == nil {
		return nil, fmt.Errorf("body cannot be nil")
	}

	bodyReader := bytes.NewReader(*body)
	req, err := http.NewRequest(http.MethodPut, getURLPath(baseURL, path), bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Host = hostname

	return sendRequest(t, req, expectedStatusCode)
}

func sendDeleteRequest(t *testing.T, hostname, baseURL, path string, expectedStatusCode int) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodDelete, getURLPath(baseURL, path), nil)
	if err != nil {
		return nil, err
	}
	req.Host = hostname

	return sendRequest(t, req, expectedStatusCode)
}

func getURLPath(baseURL, path string) string {
	return strings.TrimSuffix(baseURL, "/") + "/" + strings.TrimPrefix(path, "/")
}
