/*
Copyright 2026 The Radius Authors.

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

package bicep

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armdeployments/v3"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/driver"
	"github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/stretchr/testify/require"
)

type recoveryDeploymentClient struct {
	clients.ResourceDeploymentsClient
	get func(context.Context, string) (clients.ClientCreateOrUpdateResponse, error)
}

func (c *recoveryDeploymentClient) Get(ctx context.Context, id, _ string) (clients.ClientCreateOrUpdateResponse, error) {
	return c.get(ctx, id)
}

func TestBicepRecoveryObservesNativeOperationWithoutSubmission(t *testing.T) {
	t.Parallel()
	const resourceID = "/planes/radius/local/resourceGroups/rg/providers/Test.Compute/containers/target"
	for _, state := range []string{"Succeeded", "Running", "RunningThenSucceeded", "missing", "Failed", "unknown"} {
		t.Run(state, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var reads atomic.Int32
			d := &bicepDriver{DeploymentClient: &recoveryDeploymentClient{get: func(_ context.Context, id string) (clients.ClientCreateOrUpdateResponse, error) {
				reads.Add(1)
				require.Contains(t, id, "/deployments/recipeoriginal-operation")
				if state == "missing" {
					return clients.ClientCreateOrUpdateResponse{}, &azcore.ResponseError{StatusCode: http.StatusNotFound}
				}
				if state == "Running" {
					cancel()
				}
				nativeState := armdeployments.ProvisioningState(state)
				if state == "RunningThenSucceeded" {
					nativeState = armdeployments.ProvisioningState("Succeeded")
					if reads.Load() == 1 {
						nativeState = armdeployments.ProvisioningState("Running")
					}
				}
				return clients.ClientCreateOrUpdateResponse{DeploymentExtended: armdeployments.DeploymentExtended{
					Properties: &armdeployments.DeploymentPropertiesExtended{
						ProvisioningState: &nativeState,
						Outputs: map[string]any{"result": map[string]any{"value": map[string]any{
							"values": map[string]any{"endpoint": "retained"},
						}}},
					},
				}}, nil
			}}}
			output, err := d.Execute(ctx, driver.ExecuteOptions{BaseOptions: driver.BaseOptions{
				Recipe: recipes.ResourceMetadata{ResourceID: resourceID, ExecutionID: "original-operation", RecoverExecution: true},
			}})
			expectedReads := 1
			if state == "RunningThenSucceeded" {
				expectedReads = 2
			}
			require.EqualValues(t, expectedReads, reads.Load())
			if state == "Succeeded" || state == "RunningThenSucceeded" {
				require.NoError(t, err)
				require.Equal(t, "retained", output.Values["endpoint"])
				return
			}
			require.Error(t, err)
			require.Nil(t, output)
		})
	}
}
