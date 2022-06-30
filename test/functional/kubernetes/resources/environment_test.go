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
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/project-radius/radius/pkg/azure/clients"

	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/stretchr/testify/require"
)

const (
	resourceGroup = "default"
	apiVersion    = "2022-03-15-privatepreview"
	envName       = "my-k8s-env"
	retryInterval = 10
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
	url, roundTripper, err := kubernetes.GetBaseUrlAndRoundTripperForDeploymentEngine("", "", "", true)
	require.NoError(t, err, "")

	// Create resource group in deployments plane
	rgDeployments := fmt.Sprintf("%s/planes/deployments/local/resourceGroups/%s", url, resourceGroup)
	createRgRequestDeployments, err := http.NewRequest(
		http.MethodPut,
		rgDeployments,
		strings.NewReader(`{}`),
	)
	require.NoError(t, err, "")

	res, err := roundTripper.RoundTrip(createRgRequestDeployments)
	require.NoError(t, err, "")

	require.Equal(t, http.StatusCreated, res.StatusCode)
	t.Logf("Resource group: %s created successfully", resourceGroup)

	getRgRequestDeployments, err := http.NewRequest(
		http.MethodGet,
		rgDeployments,
		strings.NewReader(`{}`),
	)
	require.NoError(t, err, "")
	res, err = roundTripper.RoundTrip(getRgRequestDeployments)
	require.NoError(t, err, "")
	require.Equal(t, http.StatusOK, res.StatusCode)
	t.Logf("Was able to GET Resource group: %s successfully", resourceGroup)

	// Create resource group in radius plane. Currently, DE uses the same resourcegroup in the radius plane as well
	rgRadius := fmt.Sprintf("%s/planes/radius/local/resourceGroups/%s", url, resourceGroup)
	createRgRequestRadius, err := http.NewRequest(
		http.MethodPut,
		rgRadius,
		strings.NewReader(`{}`),
	)
	require.NoError(t, err, "")

	res, err = roundTripper.RoundTrip(createRgRequestRadius)
	require.NoError(t, err, "")

	require.Equal(t, http.StatusCreated, res.StatusCode)
	t.Logf("Resource group: %s created successfully", resourceGroup)

	getRgRequestRadius, err := http.NewRequest(
		http.MethodGet,
		rgRadius,
		nil,
	)
	require.NoError(t, err, "")

	res, err = roundTripper.RoundTrip(getRgRequestRadius)
	require.NoError(t, err, "")
	require.Equal(t, http.StatusOK, res.StatusCode)
	t.Logf("Was able to GET Resource group: %s successfully", resourceGroup)

	deploymentsClient := clients.NewResourceDeploymentClientWithBaseURI(url)

	deploymentsClient.Sender = &sender{RoundTripper: roundTripper}

	deploymentURL := fmt.Sprintf("/planes/deployments/local/resourceGroups/%s/providers/Microsoft.Resources/deployments/my-deployment", resourceGroup)
	future, err := deploymentsClient.CreateOrUpdate(ctx, deploymentURL, resources.Deployment{
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
							"location": "global",
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

	deployment, err := future.Result(deploymentsClient.DeploymentsClient)
	require.NoError(t, err, "Deployment failed")

	require.Equal(t, 200, deployment.StatusCode)

	go setupProxy(t)

	time.Sleep(100 * time.Millisecond)

	// Make an HTTP request to get env

	getURL := fmt.Sprintf("http://127.0.0.1:8001/apis/api.ucp.dev/v1alpha3/planes/radius/local/resourceGroups/%s/providers/Applications.Core/environments/%s?api-version=%s", resourceGroup, envName, apiVersion)
	fmt.Println(getURL)

	client := &http.Client{
		Timeout: time.Second * 10,
	}

	testGetEnvironment(t, client, "localhost", getURL, 200)
	t.Log("Success")
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

func testGetEnvironment(t *testing.T, client *http.Client, hostname, url string, expectedStatusCode int) {
	t.Log("Making a GET call to test if the environment was created")
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)

	req.Host = hostname

	retries := 60
	for i := 0; i < retries; i++ {
		t.Logf("making request to %s", url)
		response, err := client.Do(req)
		if err != nil {
			t.Logf("got error %s. retrying...", err.Error())
			time.Sleep(1 * time.Second)
			continue
		}

		if response.Body != nil {
			defer response.Body.Close()
		}

		if response.StatusCode != expectedStatusCode {
			t.Logf("got status: %d, wanted: %d. retrying...", response.StatusCode, expectedStatusCode)
			time.Sleep(retryInterval * time.Second)
			continue
		}

		// Encountered the correct status code
		return
	}

	require.NoError(t, fmt.Errorf("status code %d was not encountered after %d retries", expectedStatusCode, retries))
}
