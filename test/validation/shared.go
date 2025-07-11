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
package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/testcontext"
)

const (
	EnvironmentsResource = "applications.core/environments"
	ApplicationsResource = "applications.core/applications"
	GatewaysResource     = "applications.core/gateways"
	ContainersResource   = "applications.core/containers"
	VolumesResource      = "applications.core/volumes"
	SecretStoresResource = "applications.core/secretStores"

	RabbitMQQueuesResource          = "applications.messaging/rabbitMQQueues"
	DaprPubSubBrokersResource       = "applications.dapr/pubSubBrokers"
	DaprSecretStoresResource        = "applications.dapr/secretStores"
	DaprStateStoresResource         = "applications.dapr/stateStores"
	DaprConfigurationStoresResource = "applications.dapr/configurationStores"
	MongoDatabasesResource          = "applications.datastores/mongoDatabases"
	RedisCachesResource             = "applications.datastores/redisCaches"
	SQLDatabasesResource            = "applications.datastores/sqlDatabases"
	ExtendersResource               = "applications.core/extenders"
)

type RPResource struct {
	Type            string
	Name            string
	App             string
	OutputResources []OutputResourceResponse
}

// Output resource fields returned as a part of get/list response payload for Radius resources.
type OutputResourceResponse struct {
	// ID is the resource ID of the output resource.
	ID string
}

type RPResourceSet struct {
	Resources []RPResource
}

// DeleteRPResource deletes an environment or application resource depending on the type of the resource passed in, and
// returns an error if one occurs.
func DeleteRPResource(ctx context.Context, t *testing.T, cli *radcli.CLI, client clients.ApplicationsManagementClient, resource RPResource) error {
	if resource.Type == EnvironmentsResource {
		t.Logf("deleting environment: %s", resource.Name)

		var respFromCtx *http.Response
		ctxWithResp := runtime.WithCaptureResponse(ctx, &respFromCtx)

		_, err := client.DeleteEnvironment(ctxWithResp, resource.Name)
		if err != nil {
			return err
		}

		if respFromCtx.StatusCode == 204 {
			output.LogInfo("Environment '%s' does not exist or has already been deleted.", resource.Name)
		}

		return nil

		// TODO: this should probably call the CLI, but if you create an
		// environment via bicep deployment, it will not be reflected in the
		// rad config.
		// return cli.EnvDelete(ctx, resource.Name)
	} else if resource.Type == ApplicationsResource {
		t.Logf("deleting application: %s", resource.Name)
		return cli.ApplicationDelete(ctx, resource.Name)
	} else {
		// Handle other resource types (like ExtendersResource, ContainersResource, etc.)
		t.Logf("deleting resource: %s of type: %s", resource.Name, resource.Type)
		
		// Retry deletion with exponential backoff for 409 Conflict errors
		// Resources may be stuck in "Updating" state after failed deployments
		maxRetries := 5
		var err error
		for attempt := 0; attempt < maxRetries; attempt++ {
			_, err = client.DeleteResource(ctx, resource.Type, resource.Name)
			if err == nil {
				break
			}
			
			// Check if it's a 409 Conflict error (resource is updating)
			if strings.Contains(err.Error(), "409") && strings.Contains(err.Error(), "Conflict") {
				if attempt < maxRetries-1 {
					waitTime := time.Duration(1<<attempt) * time.Second // Exponential backoff: 1s, 2s, 4s, 8s, 16s
					t.Logf("resource %s is updating (409 Conflict), retrying in %v (attempt %d/%d)", resource.Name, waitTime, attempt+1, maxRetries)
					time.Sleep(waitTime)
					continue
				} else {
					t.Logf("resource %s still updating after %d attempts, giving up", resource.Name, maxRetries)
				}
			}
			break
		}
		return err
	}
}

// DeleteRPResourceSilent deletes an environment or application resource without logging to the test.
// This is useful for background cleanup operations to avoid "Log in goroutine after test has completed" panics.
func DeleteRPResourceSilent(ctx context.Context, cli *radcli.CLI, client clients.ApplicationsManagementClient, resource RPResource) error {
	if resource.Type == EnvironmentsResource {
		var respFromCtx *http.Response
		ctxWithResp := runtime.WithCaptureResponse(ctx, &respFromCtx)

		_, err := client.DeleteEnvironment(ctxWithResp, resource.Name)
		if err != nil {
			return err
		}

		return nil
	} else if resource.Type == ApplicationsResource {
		return cli.ApplicationDelete(ctx, resource.Name)
	} else {
		// Handle other resource types (like ExtendersResource, ContainersResource, etc.)
		
		// Retry deletion with exponential backoff for 409 Conflict errors
		// Resources may be stuck in "Updating" state after failed deployments
		maxRetries := 5
		var err error
		for attempt := 0; attempt < maxRetries; attempt++ {
			_, err = client.DeleteResource(ctx, resource.Type, resource.Name)
			if err == nil {
				break
			}
			
			// Check if it's a 409 Conflict error (resource is updating)
			if strings.Contains(err.Error(), "409") && strings.Contains(err.Error(), "Conflict") {
				if attempt < maxRetries-1 {
					waitTime := time.Duration(1<<attempt) * time.Second // Exponential backoff: 1s, 2s, 4s, 8s, 16s
					time.Sleep(waitTime)
					continue
				}
			}
			break
		}
		return err
	}
}

// ValidateRPResources checks if the expected resources exist in the response and validates the output resources if present.
func ValidateRPResources(ctx context.Context, t *testing.T, expected *RPResourceSet, client clients.ApplicationsManagementClient) {
	for _, expectedResource := range expected.Resources {
		if expectedResource.Type == EnvironmentsResource {
			envs, err := client.ListEnvironments(ctx)
			require.NoError(t, err)
			require.NotEmpty(t, envs)

			found := false
			for _, env := range envs {
				if *env.Name == expectedResource.Name {
					found = true
					break
				}
			}

			require.True(t, found, fmt.Sprintf("environment %s was not found", expectedResource.Name))
		} else if expectedResource.Type == ApplicationsResource {
			apps, err := client.ListApplications(ctx)
			require.NoError(t, err)
			require.NotEmpty(t, apps)

			found := false
			for _, app := range apps {
				if *app.Name == expectedResource.Name {
					found = true
					break
				}
			}

			require.True(t, found, fmt.Sprintf("application %s was not found", expectedResource.Name))
		} else {
			res, err := client.GetResource(ctx, expectedResource.Type, expectedResource.Name)
			require.NoError(t, err)
			require.NotNil(t, res, "resource %s with type %s does not exist", expectedResource.Name, expectedResource.Type)

			if expectedResource.App != "" {
				require.True(t, strings.HasSuffix(res.Properties["application"].(string), expectedResource.App), "resource %s with type %s was not found in application %s", expectedResource.Name, expectedResource.Type, expectedResource.App)
			}

			// Validate expected output resources are present in the response
			if len(expectedResource.OutputResources) > 0 {
				t.Log("validating output resources")
				status := res.Properties["status"].(map[string]interface{})
				or := status["outputResources"].([]interface{})
				bytes, err := json.Marshal(or)
				require.NoError(t, err)

				var outputResources []OutputResourceResponse
				err = json.Unmarshal(bytes, &outputResources)
				require.NoError(t, err)
				for _, outputResource := range outputResources {
					t.Logf("Found output resource: %+v", outputResource)
				}
				require.Equalf(t, len(expectedResource.OutputResources), len(outputResources), "expected output resources: %v, actual: %v", expectedResource.OutputResources, outputResources)
				for _, expectedOutputResource := range expectedResource.OutputResources {
					found := false
					for _, actualOutputResource := range outputResources {
						if strings.EqualFold(expectedOutputResource.ID, actualOutputResource.ID) {
							found = true
							break
						}
					}

					require.Truef(t, found, "expected output resource %v wasn't found", expectedOutputResource)
				}
			}
		}
	}
}

// AssertCredentialExists checks if the credential is registered in the workspace and returns a boolean value.
func AssertCredentialExists(t *testing.T, credential string) bool {
	ctx := testcontext.New(t)

	config, err := cli.LoadConfig("")
	require.NoError(t, err, "failed to read radius config")

	workspace, err := cli.GetWorkspace(config, "")
	require.NoError(t, err, "failed to read default workspace")
	require.NotNil(t, workspace, "default workspace is not set")

	t.Logf("Loaded workspace: %s (%s)", workspace.Name, workspace.FmtConnection())

	credentialsClient, err := connections.DefaultFactory.CreateCredentialManagementClient(ctx, *workspace)
	require.NoError(t, err, "failed to create credentials client")
	cred, err := credentialsClient.Get(ctx, credential)
	require.NoError(t, err, "failed to get credentials")

	return cred.CloudProviderStatus.Enabled
}
