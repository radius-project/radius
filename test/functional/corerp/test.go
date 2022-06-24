// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package corerp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/Azure/go-autorest/autorest"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"

	"github.com/project-radius/radius/test"
	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

var radiusControllerLogSync sync.Once

const (
	ContainerLogPathEnvVar = "RADIUS_CONTAINER_LOG_PATH"
	APIVersion             = "2022-03-15-privatepreview"
	ResourceGroup          = "default"

	retryTimeout = 2 * time.Minute
	retryBackoff = 1 * time.Second
)

type TestStep struct {
	Executor  step.Executor
	Resources []validation.Resource
}

type ApplicationTest struct {
	Options          TestOptions
	Application      string
	Description      string
	InitialResources []unstructured.Unstructured
	Steps            []TestStep
	PostDeleteVerify func(ctx context.Context, t *testing.T, at ApplicationTest)
}

type TestOptions struct {
	test.TestOptions
	DiscoveryClient discovery.DiscoveryInterface
}

func NewTestOptions(t *testing.T) TestOptions {
	return TestOptions{TestOptions: test.NewTestOptions(t)}
}

func NewApplicationTest(t *testing.T, application string, steps []TestStep, initialResources ...unstructured.Unstructured) ApplicationTest {
	return ApplicationTest{
		Options:          NewTestOptions(t),
		Application:      application,
		Description:      application,
		InitialResources: initialResources,
		Steps:            steps,
	}
}

func (at ApplicationTest) Test(t *testing.T) {
	ctx, cancel := test.GetContext(t)
	defer cancel()

	// Capture all logs from all pods (only run one of these as it will monitor everything)
	// This runs each application deployment step as a nested test, with the cleanup as part of the surrounding test.
	// This way we can catch deletion failures and report them as test failures.

	// Each of our tests are isolated to a single application, so they can run in parallel.
	t.Parallel()

	logPrefix := os.Getenv(ContainerLogPathEnvVar)
	if logPrefix == "" {
		logPrefix = "./logs"
	}

	// Only start capturing controller logs once.
	radiusControllerLogSync.Do(func() {
		err := validation.SaveLogsForController(ctx, at.Options.K8sClient, "radius-system", logPrefix)
		if err != nil {
			t.Errorf("failed to capture logs from radius controller: %v", err)
		}
	})

	err := validation.SaveLogsForApplication(ctx, at.Options.K8sClient, at.Application, logPrefix+"/"+at.Application, at.Application)
	if err != nil {
		t.Errorf("failed to capture logs from radius pods %v", err)
	}

	// Inside the integration test code we rely on the context for timeout/cancellation functionality.
	// We expect the caller to wire this out to the test timeout system, or a stricter timeout if desired.

	require.GreaterOrEqual(t, len(at.Steps), 1, "at least one step is required")

	success := true
	for i, step := range at.Steps {
		success = t.Run(step.Executor.GetDescription(), func(t *testing.T) {
			if !success {
				t.Skip("skipping due to previous step failure")
				return
			}

			t.Logf("running step %d of %d: %s", i, len(at.Steps), step.Executor.GetDescription())
			step.Executor.Execute(ctx, t, at.Options.TestOptions)
			t.Logf("finished running step %d of %d: %s", i, len(at.Steps), step.Executor.GetDescription())

			// TODO: re-enable resource validation
			// For now, test this manually by setting up a kubectl proxy
			// and querying UCP to see if these resources were created

			go setupProxy(t)
			time.Sleep(100 * time.Millisecond)

			for _, resource := range step.Resources {
				path := fmt.Sprintf("apis/api.ucp.dev/v1alpha3/planes/radius/local/resourceGroups/%s/providers/Applications.Core/%s/%s?api-version=%s", ResourceGroup, resource.Type, resource.Name, APIVersion)
				testHTTPEndpoint(t, "http://127.0.0.1:8001", path, 200)
			}

		})
	}

	// TODO: re-enable cleanup of application
}

func setupProxy(t *testing.T) {
	t.Log("Setting up kubectl proxy")
	proxyCmd := exec.Command("kubectl", "proxy", "--port", "8001")
	// Not checking the return value since ignore if already running proxy
	err := proxyCmd.Run()
	if err != nil {
		t.Logf("Failed to setup proxy with error: %v", err)
	}
	t.Log("Done setting up kubectl proxy")
}

// testHTTPEndpoint makes requests to the given baseURL/path with retries
// until it finds the desired status code (expectedStatusCode) or times out (retryTimeout)
func testHTTPEndpoint(t *testing.T, baseURL, path string, expectedStatusCode int) error {
	req, err := autorest.Prepare(&http.Request{},
		autorest.WithBaseURL(baseURL),
		autorest.WithPath(path))
	if err != nil {
		return err
	}

	// Send requests to backing container via port-forward
	response, err := autorest.Send(req,
		autorest.WithLogging(functional.NewTestLogger(t)),
		autorest.DoErrorUnlessStatusCode(expectedStatusCode),
		autorest.DoRetryForDuration(retryTimeout, retryBackoff))
	if err != nil {
		return err
	}

	if response.Body != nil {
		defer response.Body.Close()
	}

	if response.StatusCode != expectedStatusCode {
		return errors.New("did not encounter correct status code")
	}

	// Encountered the correct status code
	return nil
}
