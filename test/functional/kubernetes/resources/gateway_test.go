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
				// Setup service port-forwarding
				localHostname := "localhost"
				localPort := 8888
				readyChan := make(chan struct{}, 1)
				stopChan := make(chan struct{}, 1)
				errorChan := make(chan error)

				go functional.ExposeIngress(ctx, at.Options.K8sClient, at.Options.K8sConfig, localHostname, localPort, readyChan, stopChan, errorChan)

				// Send requests to backing container via port-forward
				url := fmt.Sprintf("http://%s:%d", localHostname, localPort)
				rewrittenURL := url + "/rewriteme"
				invalidURL := url + "/backend"

				<-readyChan
				client := &http.Client{
					Timeout: time.Second * 10,
				}

				testRequest(t, client, application, url, 200)
				testRequest(t, client, application, rewrittenURL, 200)
				testRequest(t, client, application, invalidURL, 404)
			},
		},
	})
	test.Test(t)
}

func testRequest(t *testing.T, client *http.Client, application string, url string, expectedStatusCode int) {
	req, err := http.NewRequest(http.MethodGet, url+"/healthz", nil)
	require.NoError(t, err)

	req.Host = application

	res, err := client.Do(req)
	require.NoError(t, err)

	defer res.Body.Close()

	if res.StatusCode == http.StatusNotFound {
		// Retry with localhost to support local dev tests
		localReq, err := http.NewRequest(http.MethodGet, url+"/healthz", nil)
		require.NoError(t, err)

		localReq.Host = "localhost"

		localRes, err := client.Do(localReq)
		require.NoError(t, err)

		defer localRes.Body.Close()

		require.Equal(t, expectedStatusCode, localRes.StatusCode)
		return
	}

	require.Equal(t, expectedStatusCode, res.StatusCode)
}
