// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

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
	retryTimeout = 5
)

func Test_Gateway(t *testing.T) {
	template := "testdata/kubernetes-resources-gateway.bicep"
	application := "kubernetes-resources-gateway"
	test := kubernetes.NewApplicationTest(t, application, []kubernetes.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
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
				// Get hostname from root HTTPProxy
				// Note: this just gets the hostname from the first root proxy
				// that it finds. Testing multiple gateways here will not work.
				hostname, err := functional.GetHostnameForHTTPProxy(ctx, at.Options.Client)
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
				baseURL := fmt.Sprintf("http://%s:%d", localHostname, localPort)
				client := &http.Client{
					Timeout: time.Second * 10,
				}

				var success bool
				retries := 3
			retryLoop:
				for i := 1; i <= 3; i++ {
					t.Logf("Setting up portforward (attempt %d/%d)", i, retries)
					stopChan := make(chan struct{})
					readyChan := make(chan struct{})
					errorChan := make(chan error)

					go functional.ExposeIngress(ctx, at.Options.K8sClient, at.Options.K8sConfig, localHostname, localPort, remotePort, stopChan, readyChan, errorChan)

					time.Sleep(100 * time.Millisecond)

					select {
					case err := <-errorChan:
						t.Logf("Portforward failed with error: %s. retrying: (%d/%d)", err, i, retries)
					case <-readyChan:
						t.Logf("Portforward session active at %s", baseURL)
						// Wait for a few seconds to ensure that envoy registers
						// the dynamic configuration from httpproxies on the cluster
						time.Sleep(5 * time.Second)

						if err = testGatewayAvailability(t, client, hostname, baseURL+"/healthz", 200); err != nil {
							continue retryLoop
						}

						// Both of these URLs route to the same backend service,
						// but /backend2 maps to / which allows it to access /healthz
						if err = testGatewayAvailability(t, client, hostname, baseURL+"/backend2/healthz", 200); err != nil {
							continue retryLoop
						}

						if err = testGatewayAvailability(t, client, hostname, baseURL+"/backend1/healthz", 404); err != nil {
							continue retryLoop
						}

						// All of the requests were successful
						t.Logf("All requests encountered the correct status code")
						success = true
						break retryLoop
					}
				}

				require.True(t, success, "Portforward failed to serve requests after %d retries", retries)
			},
		},
	})
	test.Test(t)
}

func testGatewayAvailability(t *testing.T, client *http.Client, hostname, url string, expectedStatusCode int) error {
	// Send requests to backing container via port-forward

	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)

	req.Host = hostname

	retries := 3
	for i := 1; i <= retries; i++ {
		t.Logf("Making request to %s...", url)
		response, err := client.Do(req)
		if err != nil {
			t.Logf("got error %s", err.Error())
			time.Sleep(retryTimeout * time.Second)
			continue
		}

		if response.Body != nil {
			defer response.Body.Close()
		}

		if response.StatusCode != expectedStatusCode {
			t.Logf("Got status: %d, wanted: %d. retrying (%d/%d)", response.StatusCode, expectedStatusCode, i, retries)
			time.Sleep(retryTimeout * time.Second)
			continue
		}

		// Encountered the correct status code
		t.Logf("Successful request: got status: %d, wanted: %d.", response.StatusCode, expectedStatusCode)
		return nil
	}

	err = fmt.Errorf("Status code %d was not encountered after %d retries", expectedStatusCode, retries)
	t.Logf(err.Error())
	return err
}
