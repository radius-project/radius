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
	"github.com/project-radius/radius/test/kubernetestest"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
)

const (
	retryTimeout = 10
)

func Test_Gateway(t *testing.T) {
	template := "testdata/kubernetes-resources-gateway.bicep"
	application := "kubernetes-resources-gateway"
	test := kubernetestest.NewApplicationTest(t, application, []kubernetestest.Step{
		{
			Executor: kubernetestest.NewDeployStepExecutor(template),
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
			PostStepVerify: func(ctx context.Context, t *testing.T, at kubernetestest.ApplicationTest) {
				// Get hostname from root HTTPProxy
				// Note: this just gets the hostname from the first root proxy
				// that it finds. Testing multiple gateways here will not work.
				hostname, err := functional.GetHostnameForHTTPProxy(ctx, at.Options.Client)
				require.NoError(t, err)

				// Setup service port-forwarding
				localHostname := "localhost"
				localPort := 8888
				readyChan := make(chan struct{}, 1)
				stopChan := make(chan struct{}, 1)
				errorChan := make(chan error)

				go functional.ExposeIngress(ctx, at.Options.K8sClient, at.Options.K8sConfig, localHostname, localPort, readyChan, stopChan, errorChan)

				// Send requests to backing container via port-forward
				baseURL := fmt.Sprintf("http://%s:%d", localHostname, localPort)

				<-readyChan
				client := &http.Client{
					Timeout: time.Second * 10,
				}

				testRequest(t, client, hostname, baseURL+"/healthz", 200)

				// Both of these URLs route to the same backend service,
				// but /backend2 maps to / which allows it to access /healthz
				testRequest(t, client, hostname, baseURL+"/backend2/healthz", 200)
				testRequest(t, client, hostname, baseURL+"/backend1/healthz", 404)
			},
		},
	})
	test.Test(t)
}

func testRequest(t *testing.T, client *http.Client, hostname, url string, expectedStatusCode int) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)

	req.Host = hostname

	retries := 5
	for i := 0; i < retries; i++ {
		t.Logf("making request to %s", url)
		response, err := client.Do(req)
		if err != nil {
			t.Logf("got error %s", err.Error())
			time.Sleep(1 * time.Second)
			continue
		}

		if response.Body != nil {
			defer response.Body.Close()
		}

		if response.StatusCode != expectedStatusCode {
			t.Logf("got status: %d, wanted: %d. retrying...", response.StatusCode, expectedStatusCode)
			time.Sleep(retryTimeout * time.Second)
			continue
		}

		// Encountered the correct status code
		return
	}

	require.NoError(t, fmt.Errorf("status code %d was not encountered after %d retries", expectedStatusCode, retries))
}
