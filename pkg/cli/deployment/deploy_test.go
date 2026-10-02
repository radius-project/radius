/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armdeployments"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/clients"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	ucpresources "github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

func Test_GetProviderConfigs(t *testing.T) {
	resourceDeploymentClient := ResourceDeploymentClient{
		RadiusResourceGroup: "testrg",
	}
	options := clients.DeploymentOptions{
		Providers: &clients.Providers{},
	}

	var expectedConfig sdkclients.ProviderConfig

	expectedConfig.Radius = &sdkclients.Radius{
		Type: "Radius",
		Value: sdkclients.Value{
			Scope: "/planes/radius/local/resourceGroups/" + "testrg",
		},
	}
	expectedConfig.Deployments = &sdkclients.Deployments{
		Type: "Microsoft.Resources",
		Value: sdkclients.Value{
			Scope: "/planes/radius/local/resourceGroups/" + "testrg",
		},
	}

	providerConfig := resourceDeploymentClient.GetProviderConfigs(options)
	require.Equal(t, providerConfig, expectedConfig)
}

func Test_GetProviderConfigsWithAzProvider(t *testing.T) {
	resourceDeploymentClient := ResourceDeploymentClient{
		RadiusResourceGroup: "testrg",
	}

	options := clients.DeploymentOptions{
		Providers: &clients.Providers{
			Azure: &clients.AzureProvider{
				Scope: "/subscriptions/dummy/resourceGroups/azrg",
			},
		},
	}

	var expectedConfig sdkclients.ProviderConfig

	expectedConfig.Az = &sdkclients.Az{
		Type: "AzureResourceManager",
		Value: sdkclients.Value{
			Scope: "/subscriptions/dummy/resourceGroups/" + "azrg",
		},
	}

	expectedConfig.Radius = &sdkclients.Radius{
		Type: "Radius",
		Value: sdkclients.Value{
			Scope: "/planes/radius/local/resourceGroups/" + "testrg",
		},
	}
	expectedConfig.Deployments = &sdkclients.Deployments{
		Type: "Microsoft.Resources",
		Value: sdkclients.Value{
			Scope: "/planes/radius/local/resourceGroups/" + "testrg",
		},
	}

	providerConfig := resourceDeploymentClient.GetProviderConfigs(options)
	require.Equal(t, providerConfig, expectedConfig)
}

func Test_ResourceDeploymentClient_MonitorProgress_CanonicalIDs(t *testing.T) {
	t.Parallel()

	cacheID, err := ucpresources.ParseResource("/planes/radius/local/resourceGroups/testrg/providers/Radius.Data/redisCaches/cache")
	require.NoError(t, err)
	appID, err := ucpresources.ParseResource("/planes/radius/local/resourceGroups/testrg/providers/Radius.Core/applications/app")
	require.NoError(t, err)

	// Exercise the real operations client and polling consumer against an in-memory transport.
	// Virtual time advances the production polling interval without wall-clock sleeps.
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()

		states := [][2]string{
			{"Pending", "Running"},
			{"Running", "Running"},
			{"Failed", "Succeeded"},
		}
		polls := 0
		transport := deploymentOperationsTransport(func(req *http.Request) (*http.Response, error) {
			require.Equal(t, http.MethodGet, req.Method)
			require.Equal(t, "/planes/radius/local/resourcegroups/testrg/providers/Microsoft.Resources/deployments/test-deployment/operations", req.URL.Path)
			require.Equal(t, sdkclients.DeploymentOperationsClientAPIVersion, req.URL.Query().Get("api-version"))
			// Cancel the next request, after the last successful response has reached the progress channel.
			if polls == len(states) {
				cancel()
				return nil, ctx.Err()
			}
			require.Less(t, polls, len(states), "unexpected extra operations poll")

			state := states[polls]
			body, err := json.Marshal(armdeployments.DeploymentOperationsListResult{
				Value: []*armdeployments.DeploymentOperation{
					{Properties: &armdeployments.DeploymentOperationProperties{
						ProvisioningState: new(state[0]),
						TargetResource:    &armdeployments.TargetResource{ID: new(cacheID.String())},
					}},
					{Properties: &armdeployments.DeploymentOperationProperties{
						ProvisioningState: new(state[1]),
						TargetResource:    &armdeployments.TargetResource{ID: new(appID.String())},
					}},
					{Properties: &armdeployments.DeploymentOperationProperties{
						ProvisioningState: new("Failed"),
						TargetResource:    &armdeployments.TargetResource{ResourceName: new("symbolic-only")},
					}},
				},
			})
			require.NoError(t, err)
			polls++

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(string(body))),
				Request:    req,
			}, nil
		})
		operationsClient, err := sdkclients.NewResourceDeploymentOperationsClient(&sdkclients.Options{
			BaseURI: "https://radius.test",
			Cred:    &aztoken.AnonymousCredential{},
			ARMClientOptions: &arm.ClientOptions{
				Transport: transport,
				Retry:     policy.RetryOptions{MaxRetries: -1},
			},
		})
		require.NoError(t, err)

		client := ResourceDeploymentClient{RadiusResourceGroup: "testrg", OperationsClient: operationsClient}
		progress := make(chan clients.ResourceProgress, len(states)*3)
		var wg sync.WaitGroup
		require.NoError(t, client.monitorProgress(ctx, "test-deployment", progress, &wg))
		wg.Wait()
		close(progress)

		var updates []clients.ResourceProgress
		for update := range progress {
			updates = append(updates, update)
		}
		require.Equal(t, len(states), polls)
		require.Equal(t, []clients.ResourceProgress{
			{Resource: cacheID, Status: clients.StatusStarted},
			{Resource: appID, Status: clients.StatusStarted},
			{Resource: cacheID, Status: clients.StatusFailed},
			{Resource: appID, Status: clients.StatusCompleted},
		}, updates, "running polls must not duplicate Started updates or invent IDs for symbolic-only operations")
	})

	// A failed resource progress event is not the deployment's terminal result.
	// That result, including its error and outputs, comes from the separate deployment poller.
	deploymentErr := errors.New("deployment failed")
	for _, tt := range []struct {
		name string
		err  error
	}{
		{name: "deployment succeeded"},
		{name: "deployment failed", err: deploymentErr},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := sdkclients.NewMockResourceDeploymentsClient()
			client := ResourceDeploymentClient{RadiusResourceGroup: "testrg", Client: mockClient}
			poller, err := client.startDeployment(t.Context(), "test-deployment", clients.DeploymentOptions{})
			require.NoError(t, err)
			token, err := poller.ResumeToken()
			require.NoError(t, err)
			mockClient.CompleteOperation(token, func(state *sdkclients.OperationState) {
				state.Err = tt.err
				state.Value = sdkclients.ClientCreateOrUpdateResponse{
					DeploymentExtended: armdeployments.DeploymentExtended{
						Properties: &armdeployments.DeploymentPropertiesExtended{
							OutputResources: []*armdeployments.ResourceReference{{ID: new(cacheID.String())}},
							Outputs: map[string]any{
								"cacheName": map[string]any{"type": "string", "value": cacheID.Name()},
							},
						},
					},
				}
			})

			result, err := client.waitForCompletion(t.Context(), poller)
			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
				require.Empty(t, result)
				return
			}

			require.NoError(t, err)
			require.Equal(t, clients.DeploymentResult{
				Resources: []ucpresources.ID{cacheID},
				Outputs:   map[string]clients.DeploymentOutput{"cacheName": {Type: "string", Value: cacheID.Name()}},
			}, result)
		})
	}
}

type deploymentOperationsTransport func(*http.Request) (*http.Response, error)

func (transport deploymentOperationsTransport) Do(req *http.Request) (*http.Response, error) {
	return transport(req)
}
