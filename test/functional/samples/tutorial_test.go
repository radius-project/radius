package samples

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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

// What needs to happen
// Need to add k8s resources from sample from radius
// need to be able to run locally
// Remove submodule and ask user to point to repo directory
//

func Test_TutorialSampleMongoContainer(t *testing.T) {
	template := "samples/tutorial/app.bicep"
	appName := "webapp"

	requiredSecrets := map[string]map[string]string{}

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
				hostname, err := functional.GetHostnameForHTTPProxy(ctx, ct.Options.Client, "default", appName)
				require.NoError(t, err)
				t.Logf("found root proxy with hostname: {%s}", hostname)

				// Set up pod port-forwarding for contour-envoy
				t.Run("check gwy", func(t *testing.T) {
					for i := 1; i <= retries; i++ {
						t.Logf("Setting up portforward (attempt %d/%d)", i, retries)
						err = testGatewayWithPortForward(t, ctx, ct, hostname, remotePort, retries)
						if err != nil {
							t.Logf("Failed to test Gateway via portforward with error: %s", err)
						} else {
							// Successfully ran tests
							return
						}
					}
				})
			},
			// TODO: validation of k8s resources created by mongo-container is blocked by https://github.com/Azure/bicep-extensibility/issues/88
			// TODO: https://github.com/project-radius/radius/issues/4689, validation doesn't work correctly today
			K8sOutputResources: []unstructured.Unstructured{},
			// Application and Environment should not render any K8s Objects directly
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(appName, "frontend"),
						validation.NewK8sHTTPProxyForResource(appName, "public"),
						validation.NewK8sHTTPProxyForResource(appName, "http-route"),
						validation.NewK8sServiceForResource(appName, "http-route"),
					},
				},
			},
		},
	}, requiredSecrets)

	test.Test(t)
}

func testGatewayWithPortForward(t *testing.T, ctx context.Context, at corerp.CoreRPTest, hostname string, remotePort, retries int) error {
	// stopChan will close the port-forward connection on close
	stopChan := make(chan struct{})

	// portChan will be populated with the assigned port once the port-forward connection is opened on it
	portChan := make(chan int)

	// errorChan will contain any errors created from initializing the port-forwarding session
	errorChan := make(chan error)

	go functional.ExposeIngress(t, ctx, at.Options.K8sClient, at.Options.K8sConfig, remotePort, stopChan, portChan, errorChan)

	select {
	case err := <-errorChan:
		return fmt.Errorf("portforward failed with error: %s", err)
	case localPort := <-portChan:
		baseURL := fmt.Sprintf("http://localhost:%d", localPort)
		t.Logf("Portforward session active at %s", baseURL)

		if err := testGatewayAvailability(t, hostname, baseURL, "", 200); err != nil {
			close(stopChan)
			return err
		}

		// All of the requests were successful
		t.Logf("All requests encountered the correct status code")
		return nil
	}
}

func testGatewayAvailability(t *testing.T, hostname, baseURL, path string, expectedStatusCode int) error {
	// Using autorest as an http client library because of its retry capabilities
	req, err := autorest.Prepare(&http.Request{},
		autorest.WithBaseURL(baseURL),
		autorest.WithPath(path))
	if err != nil {
		return err
	}

	req.Host = hostname

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
