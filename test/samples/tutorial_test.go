package samples

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Azure/go-autorest/autorest"
	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
)

const (
	remotePort   = 8080
	retries      = 3
	retryTimeout = 1 * time.Minute
	retryBackoff = 1 * time.Second
)

func Test_TutorialSample(t *testing.T) {
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

				t.Run("check gwy", func(t *testing.T) {
					err = testGatewayAvailability(t, hostname, "localhost", "/", http.StatusOK)
					if err != nil {
						t.Logf("Failed to test Gateway via portforward with error: %s", err)
					} else {
						// Successfully ran tests
						return
					}
				})
			},
			// Application and Environment should not render any K8s Objects directly
			K8sObjects: &validation.K8sObjectSet{
				// TODO: validation of k8s resources created in the module is blocked by https://github.com/Azure/bicep-extensibility/issues/88
			},
		},
	}, requiredSecrets)

	test.Test(t)
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
