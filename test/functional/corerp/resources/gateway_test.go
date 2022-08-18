// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

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
)

const (
	retryTimeout = 1 * time.Minute
	retryBackoff = 1 * time.Second
)

func Test_Gateway(t *testing.T) {
	template := "testdata/corerp-resources-gateway.bicep"
	name := "corerp-resources-gateway"

	requiredSecrets := map[string]map[string]string{}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "gtwy-gtwy",
						Type: validation.GatewaysResource,
						App:  name,
					},
					{
						Name: "gtwy-front-rte",
						Type: validation.HttpRoutesResource,
						App:  name,
					},
					{
						Name: "gtwy-front-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "gtwy-back-rte",
						Type: validation.HttpRoutesResource,
						App:  name,
					},
					{
						Name: "gtwy-back-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "gtwy-front-ctnr"),
						validation.NewK8sPodForResource(name, "gtwy-back-ctnr"),
						validation.NewK8sHTTPProxyForResource(name, "gtwy-gtwy"),
						validation.NewK8sHTTPProxyForResource(name, "gtwy-front-rte"),
						validation.NewK8sServiceForResource(name, "gtwy-front-rte"),
						validation.NewK8sHTTPProxyForResource(name, "gtwy-back-rte"),
						validation.NewK8sServiceForResource(name, "gtwy-back-rte"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct corerp.CoreRPTest) {
				// Get hostname from root HTTPProxy in 'default' namespace
				hostname, err := functional.GetHostnameForHTTPProxy(ctx, ct.Options.Client, "default", name)
				require.NoError(t, err)
				t.Logf("found root proxy with hostname: {%s}", hostname)

				var remotePort int
				if hostname == "localhost" {
					// contour-envoy runs on port 80 by default in local scenario
					remotePort = 80
				} else {
					remotePort = 8080
				}

				// Set up pod port-forwarding for contour-envoy
				localHostname := "localhost"
				localPort := 8888

				retries := 3
				for i := 1; i <= retries; i++ {
					t.Logf("Setting up portforward (attempt %d/%d)", i, retries)
					err = testGatewayWithPortforward(t, ctx, ct, hostname, localHostname, localPort, remotePort, retries)
					if err != nil {
						t.Logf("Failed to test Gateway via portforward with error: %s", err)
					} else {
						// Successfully ran tests
						return
					}
				}

				require.Fail(t, fmt.Sprintf("Gateway tests failed after %d retries", retries))
			},
		},
	}, requiredSecrets)

	test.Test(t)
}

func testGatewayWithPortforward(t *testing.T, ctx context.Context, at corerp.CoreRPTest, hostname, localHostname string, localPort, remotePort, retries int) error {
	stopChan := make(chan struct{})
	readyChan := make(chan struct{})
	errorChan := make(chan error)

	go functional.ExposeIngress(t, ctx, at.Options.K8sClient, at.Options.K8sConfig, localHostname, localPort, remotePort, stopChan, readyChan, errorChan)

	select {
	case err := <-errorChan:
		return fmt.Errorf("portforward failed with error: %s", err)
	case <-readyChan:
		baseURL := fmt.Sprintf("http://%s:%d", localHostname, localPort)
		t.Logf("Portforward session active at %s", baseURL)

		if err := testGatewayAvailability(t, hostname, baseURL, "healthz", 200); err != nil {
			close(stopChan)
			return err
		}

		// Both of these URLs route to the same backend service,
		// but /backend2 maps to / which allows it to access /healthz
		if err := testGatewayAvailability(t, hostname, baseURL, "backend2/healthz", 200); err != nil {
			close(stopChan)
			return err
		}

		if err := testGatewayAvailability(t, hostname, baseURL, "backend1/healthz", 404); err != nil {
			close(stopChan)
			return err
		}

		// All of the requests were successful
		t.Logf("All requests encountered the correct status code")
		return nil
	}
}

func testGatewayAvailability(t *testing.T, hostname, baseURL, path string, expectedStatusCode int) error {
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
