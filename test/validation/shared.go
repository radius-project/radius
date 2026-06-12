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
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
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

	// Radius.Core resource types (new provider).
	CoreEnvironmentsResource = "radius.core/environments"
	CoreApplicationsResource = "radius.core/applications"

	// Radius.Compute resource types (new provider).
	ComputeContainersResource = "radius.compute/containers"

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
		ctxWithResp := policy.WithCaptureResponse(ctx, &respFromCtx)

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
	} else if resource.Type == CoreApplicationsResource {
		// Radius.Core applications require cascade delete via the CLI --preview flag,
		// which deletes owned resources before deleting the application itself.
		t.Logf("deleting Radius.Core application: %s", resource.Name)
		_, err := cli.ApplicationDeletePreview(ctx, resource.Name, "")
		return err
	} else if resource.Type == CoreEnvironmentsResource {
		t.Logf("deleting Radius.Core environment: %s", resource.Name)
		_, err := cli.EnvironmentDeletePreview(ctx, resource.Name, "")
		return err
	}

	// Other resource types (containers, databases, etc.) are cleaned up
	// via cascade delete when their parent application is deleted.
	return nil
}

// DeleteRPResourceSilent deletes an environment or application resource without logging to the test.
// This is useful for background cleanup operations to avoid "Log in goroutine after test has completed" panics.
func DeleteRPResourceSilent(ctx context.Context, cli *radcli.CLI, client clients.ApplicationsManagementClient, resource RPResource) error {
	if resource.Type == EnvironmentsResource {
		var respFromCtx *http.Response
		ctxWithResp := policy.WithCaptureResponse(ctx, &respFromCtx)

		_, err := client.DeleteEnvironment(ctxWithResp, resource.Name)
		if err != nil {
			return err
		}

		return nil
	} else if resource.Type == ApplicationsResource {
		return cli.ApplicationDelete(ctx, resource.Name)
	} else if resource.Type == CoreApplicationsResource {
		// Radius.Core applications require cascade delete via the CLI --preview flag.
		_, err := cli.ApplicationDeletePreview(ctx, resource.Name, "")
		return err
	} else if resource.Type == CoreEnvironmentsResource {
		_, err := cli.EnvironmentDeletePreview(ctx, resource.Name, "")
		return err
	} else {
		// Handle other resource types (like ExtendersResource, ContainersResource, etc.)
		// Use force=true to handle resources that may be stuck in non-terminal provisioning states
		// (e.g., "Updating" after failed deployments), which would otherwise return 409 Conflict.
		_, err := client.DeleteResource(ctx, resource.Type, resource.Name, true)
		return err
	}
}

// ValidateRPResources checks if the expected resources exist in the response and validates the output resources if present.
func ValidateRPResources(ctx context.Context, t *testing.T, expected *RPResourceSet, client clients.ApplicationsManagementClient) {
	for _, expectedResource := range expected.Resources {
		if expectedResource.Type == EnvironmentsResource {
			// Use GetEnvironment instead of ListEnvironments to avoid a read-after-write
			// race where the resource exists but is not yet visible via the paginated list.
			_, err := client.GetEnvironment(ctx, expectedResource.Name)
			require.NoErrorf(t, err, "environment %s was not found", expectedResource.Name)
		} else if expectedResource.Type == ApplicationsResource {
			// Use GetApplication instead of ListApplications to avoid a read-after-write
			// race where the resource exists but is not yet visible via the paginated list.
			_, err := client.GetApplication(ctx, expectedResource.Name)
			require.NoErrorf(t, err, "application %s was not found", expectedResource.Name)
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
				status := res.Properties["status"].(map[string]any)
				or := status["outputResources"].([]any)
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
//
// Local-dev escape hatch: when RADIUS_TEST_USE_LOCAL_CLOUD_CREDS lists the credential
// (comma-separated; supported values: "azure", "aws", or "1"/"true" for all clouds),
// the UCP credential check is bypassed and the test is allowed to run as if the
// credential were registered. This is used by the `test-functional-azure-local`
// make target, which runs Radius components with ambient cloud credentials
// (az CLI / AWS profile) instead of credentials registered in UCP. The container-DE
// path used by CI and most contributors is unaffected.
func AssertCredentialExists(t *testing.T, credential string) bool {
	if localCloudCredAllowed(credential) {
		t.Logf("RADIUS_TEST_USE_LOCAL_CLOUD_CREDS includes %q; bypassing UCP credential check", credential)
		return true
	}

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

// localCloudCredAllowed reports whether the given credential ("azure", "aws") is
// covered by the RADIUS_TEST_USE_LOCAL_CLOUD_CREDS escape hatch. Accepted values:
//   - "1" / "true": all clouds.
//   - comma-separated list of cloud names, e.g. "azure" or "azure,aws".
func localCloudCredAllowed(credential string) bool {
	v := strings.TrimSpace(os.Getenv("RADIUS_TEST_USE_LOCAL_CLOUD_CREDS"))
	if v == "" {
		return false
	}
	if v == "1" || strings.EqualFold(v, "true") {
		return true
	}
	for _, item := range strings.Split(v, ",") {
		if strings.EqualFold(strings.TrimSpace(item), credential) {
			return true
		}
	}
	return false
}
