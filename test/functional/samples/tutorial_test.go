// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package samples

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azure/go-autorest/autorest"
	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	remotePort   = 8080
	retries      = 3
	retryTimeout = 1 * time.Minute
	retryBackoff = 1 * time.Second
)

var samplesRepoAbsPath, samplesRepoEnvVarSet = os.LookupEnv("PROJECT_RADIUS_SAMPLES_REPO_ABS_PATH")

// Test process must run with PROJECT_RADIUS_SAMPLES_REPO_ABS_PATH env var set to samples repo absolute path
// You can set the variables used by vscode codelens (e.g. 'debug test', 'run test') using 'go.testEnvVars' in vscode settings.json
// Ex: export PROJECT_RADIUS_SAMPLES_REPO_ABS_PATH=/home/uname/src/samples
func Test_TutorialSampleMongoContainer(t *testing.T) {
	if !samplesRepoEnvVarSet {
		t.Skipf("Skip samples test execution, to enable you must set env var PROJECT_RADIUS_SAMPLES_REPO_ABS_PATH to the absolute path of the project-radius/samples repository")
	}
	cwd, _ := os.Getwd()
	relPathSamplesRepo, _ := filepath.Rel(cwd, samplesRepoAbsPath)
	template := filepath.Join(relPathSamplesRepo, "tutorial/app.bicep")
	appName := "webapp"
	appNamespace := "default-webapp"

	test := corerp.NewCoreRPTest(t, appName, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: appName,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "frontend",
						Type: validation.ContainersResource,
					},
					{
						Name: "http-route",
						Type: validation.HttpRoutesResource,
					},
					{
						Name: "public",
						Type: validation.GatewaysResource,
					},
					{
						Name: "db",
						Type: validation.MongoDatabasesResource,
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct corerp.CoreRPTest) {
				// Get hostname from root HTTPProxy in 'default' namespace
				hostname, err := functional.GetHostnameForHTTPProxy(ctx, ct.Options.Client, appNamespace, appName)
				require.NoError(t, err)
				t.Logf("found root proxy with hostname: {%s}", hostname)

				// Set up pod port-forwarding for contour-envoy
				for i := 1; i <= retries; i++ {
					t.Logf("Setting up portforward (attempt %d/%d)", i, retries)
					// TODO: simplify code logic complexity through - https://github.com/project-radius/radius/issues/4778
					err = testGatewayWithPortForward(t, ctx, ct, hostname, remotePort)
					if err != nil {
						t.Logf("Failed to test Gateway via portforward with error: %s", err)
					} else {
						// Successfully ran tests
						return
					}
				}

				require.Fail(t, fmt.Sprintf("Gateway tests failed after %d retries", retries))
			},
			// TODO: validation of k8s resources created by mongo-container is blocked by https://github.com/Azure/bicep-extensibility/issues/88
			// TODO: validation of k8s resources blocked by https://github.com/project-radius/radius/issues/4689
			K8sOutputResources: []unstructured.Unstructured{},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(appName, "frontend"),
						validation.NewK8sHTTPProxyForResource(appName, "public"),
						validation.NewK8sHTTPProxyForResource(appName, "http-route"),
						validation.NewK8sServiceForResource(appName, "http-route"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func testGatewayWithPortForward(t *testing.T, ctx context.Context, at corerp.CoreRPTest, hostname string, remotePort int) error {
	// stopChan will close the port-forward connection on close
	stopChan := make(chan struct{})

	// portChan will be populated with the assigned port once the port-forward connection is opened on it
	portChan := make(chan int)

	// errorChan will contain any errors created from initializing the port-forwarding session
	errorChan := make(chan error)

	go functional.ExposeIngress(t, ctx, at.Options.K8sClient, at.Options.K8sConfig, remotePort, stopChan, portChan, errorChan)
	defer close(stopChan)

	select {
	case err := <-errorChan:
		return fmt.Errorf("portforward failed with error: %s", err)
	case localPort := <-portChan:
		baseURL := fmt.Sprintf("http://localhost:%d", localPort)
		t.Logf("Portforward session active at %s", baseURL)

		// Test base endpoint, i.e., base URL returns a 200
		_, err := sendGetRequest(t, hostname, baseURL, "", 200)
		if err != nil {
			return err
		}

		// Test GET /api/todos (list)
		listResponse, err := sendGetRequest(t, hostname, baseURL, "api/todos", 200)
		if err != nil {
			return err
		}

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
	// Using autorest as an http client library because of its retry capabilities
	return autorest.Send(req,
		autorest.WithLogging(functional.NewTestLogger(t)),
		autorest.DoErrorUnlessStatusCode(expectedStatusCode),
		autorest.DoRetryForDuration(retryTimeout, retryBackoff))
}

func sendGetRequest(t *testing.T, hostname, baseURL, path string, expectedStatusCode int) (*http.Response, error) {
	req, err := autorest.Prepare(&http.Request{},
		autorest.AsGet(),
		autorest.WithBaseURL(baseURL),
		autorest.WithPath(path))
	if err != nil {
		return nil, err
	}

	req.Host = hostname
	return sendRequest(t, req, expectedStatusCode)
}

func sendPostRequest(t *testing.T, hostname, baseURL, path string, body *[]byte, expectedStatusCode int) (*http.Response, error) {
	req, err := autorest.Prepare(&http.Request{},
		autorest.AsPost(),
		autorest.AsJSON(),
		autorest.WithBytes(body),
		autorest.WithBaseURL(baseURL),
		autorest.WithPath(path))
	if err != nil {
		return nil, err
	}

	req.Host = hostname
	return sendRequest(t, req, expectedStatusCode)
}

func sendPutRequest(t *testing.T, hostname, baseURL, path string, body *[]byte, expectedStatusCode int) (*http.Response, error) {
	req, err := autorest.Prepare(&http.Request{},
		autorest.AsPut(),
		autorest.AsJSON(),
		autorest.WithBytes(body),
		autorest.WithBaseURL(baseURL),
		autorest.WithPath(path))
	if err != nil {
		return nil, err
	}

	req.Host = hostname
	return sendRequest(t, req, expectedStatusCode)
}

func sendDeleteRequest(t *testing.T, hostname, baseURL, path string, expectedStatusCode int) (*http.Response, error) {
	req, err := autorest.Prepare(&http.Request{},
		autorest.AsDelete(),
		autorest.WithBaseURL(baseURL),
		autorest.WithPath(path))
	if err != nil {
		return nil, err
	}

	req.Host = hostname
	return sendRequest(t, req, expectedStatusCode)
}
