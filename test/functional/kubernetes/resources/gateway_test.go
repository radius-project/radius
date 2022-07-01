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
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/kubernetes"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
)

const (
	retryTimeout = 1 * time.Minute
	retryBackoff = 1 * time.Second
)

func Test_Gateway(t *testing.T) {
	template := "testdata/kubernetes-resources-gateway.bicep"
	application := "kubernetes-resources-gateway"
	test := kubernetes.NewApplicationTest(t, application, []kubernetes.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "frontend",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, rest.ResourceType{Type: resourcekinds.Deployment, Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDService:    validation.NewOutputResource(outputresource.LocalIDService, rest.ResourceType{Type: resourcekinds.Service, Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "backend",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, rest.ResourceType{Type: resourcekinds.Deployment, Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDService:    validation.NewOutputResource(outputresource.LocalIDService, rest.ResourceType{Type: resourcekinds.Service, Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sPodForResource(application, "frontend"),
						validation.NewK8sPodForResource(application, "backend"),
						validation.NewK8sHTTPProxyForResource(application, "gateway"),
						validation.NewK8sHTTPProxyForResource(application, "frontendroute"),
						validation.NewK8sServiceForResource(application, "frontendroute"),
						validation.NewK8sHTTPProxyForResource(application, "backendroute"),
						validation.NewK8sServiceForResource(application, "backendroute"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, at kubernetes.ApplicationTest) {
				// Get hostname from root HTTPProxy in 'application' namespace
				hostname, err := functional.GetHostnameForHTTPProxy(ctx, at.Options.Client, application)
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
					err = testGatewayWithPortforward(t, ctx, at, hostname, localHostname, localPort, remotePort, retries)
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
	})
	test.Test(t)
}

func testGatewayWithPortforward(t *testing.T, ctx context.Context, at kubernetes.ApplicationTest, hostname, localHostname string, localPort, remotePort, retries int) error {
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
