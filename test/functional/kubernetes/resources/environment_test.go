// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/project-radius/radius/pkg/azure/clients"

	// "github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/stretchr/testify/require"
)

const (
	resourceGroup = "default"
	apiVersion    = "2022-03-15-privatepreview"
	envName       = "my-k8s-env"
)

type sender struct {
	RoundTripper http.RoundTripper
}

func (s *sender) Do(request *http.Request) (*http.Response, error) {
	return s.RoundTripper.RoundTrip(request)
}

// TODO: This is an interim e2e test that verifies whether a PUT deployment
// to create an environment succeeds. This test can be deleted once all the RP
// tests have been migrated to use the CoreRP
func Test_EnvironmentWithCoreRP(t *testing.T) {
	ctx := context.Background()
	url, roundTripper, err := kubernetes.GetBaseUrlAndRoundTripperForDeploymentEngine("", "", true)

	require.NoError(t, err, "")

	deploymentsClient := clients.NewResourceDeploymentClientWithBaseURI(url)

	deploymentsClient.Sender = &sender{RoundTripper: roundTripper}

	future, err := deploymentsClient.CreateOrUpdate(ctx, "/planes/deployments/local/resourceGroups/default/providers/Microsoft.Resources/deployments/my-deployment", resources.Deployment{
		Properties: &resources.DeploymentProperties{
			Mode: resources.DeploymentModeIncremental,
			Template: map[string]interface{}{
				"$schema":         "https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#",
				"languageVersion": "1.9-experimental",
				"contentVersion":  "1.0.0.0",
				"metadata": map[string]interface{}{
					"EXPERIMENTAL_WARNING": "Symbolic name support in ARM is experimental, and should be enabled for testing purposes only. Do not enable this setting for any production usage, or you may be unexpectedly broken at any time!",
					"_generator": map[string]interface{}{
						"name":         "bicep",
						"version":      "0.5.172.5655",
						"templateHash": "16021869117229921583",
					},
				},
				"imports": map[string]interface{}{
					"radius": map[string]interface{}{
						"provider": "Radius",
						"version":  "1.0",
						"config": map[string]interface{}{
							"foo": "foo",
						},
					},
				},
				"resources": map[string]interface{}{
					"env": map[string]interface{}{
						"import": "radius",
						"type":   "Applications.Core/environments@2022-03-15-privatepreview",
						"properties": map[string]interface{}{
							"name":     envName,
							"location": "westus2",
							"properties": map[string]interface{}{
								"compute": map[string]interface{}{
									"kind":       "kubernetes",
									"resourceId": "",
								},
							},
						},
					},
				},
			},
			Parameters: map[string]interface{}{},
		},
	})

	require.NoError(t, err, "Deployment failed")

	err = future.WaitForCompletionRef(ctx, deploymentsClient.Client)
	require.NoError(t, err, "Deployment failed")

	_, err = future.Result(deploymentsClient.DeploymentsClient)
	require.NoError(t, err, "Deployment failed")

	setupProxy()

	// Make an HTTP request to get env

	getURL := fmt.Sprintf("http://127.0.0.1:8001/apis/api.ucp.dev/v1alpha3/planes/radius/local/resourceGroups/%s/providers/Applications.Core/environments/%s?api-version=%s", resourceGroup, envName, apiVersion)
	fmt.Println(getURL)

	client := &http.Client{
		Timeout: time.Second * 10,
	}
	testGetEnvironment(t, client, "localhost", getURL, 200)
}

func setupProxy() {
	proxyCmd := exec.Command("kubectl", "proxy", "&")
	// Not checking the return value since ignore if already running proxy
	_ = proxyCmd.Run()
}

func testGetEnvironment(t *testing.T, client *http.Client, hostname, url string, expectedStatusCode int) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)

	req.Host = hostname

	retries := 60
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
