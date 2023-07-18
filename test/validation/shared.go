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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"

	"github.com/project-radius/radius/test/radcli"
)

const (
	EnvironmentsResource = "applications.core/environments"
	ApplicationsResource = "applications.core/applications"
	HttpRoutesResource   = "applications.core/httpRoutes"
	GatewaysResource     = "applications.core/gateways"
	ContainersResource   = "applications.core/containers"
	VolumesResource      = "applications.core/volumes"
	SecretStoresResource = "applications.core/secretStores"

	MongoDatabasesResource          = "applications.link/mongoDatabases"
	O_RabbitMQMessageQueuesResource = "applications.link/rabbitMQMessageQueues"
	RedisCachesResource             = "applications.link/redisCaches"
	SQLDatabasesResource            = "applications.link/sqlDatabases"
	DaprPubSubBrokersResource       = "applications.link/daprPubSubBrokers"
	DaprSecretStoresResource        = "applications.link/daprSecretStores"
	DaprStateStoresResource         = "applications.link/daprStateStores"
	ExtendersResource               = "applications.link/extenders"

	// New resources after splitting LinkRP namespace
	RabbitMQQueuesResource = "applications.messaging/rabbitMQQueues"
)

type RPResource struct {
	Type            string
	Name            string
	App             string
	OutputResources []OutputResourceResponse
}

// Output resource fields returned as a part of get/list response payload for Radius resources
// https://github.com/project-radius/radius/blob/main/pkg/rp/types.go#L173
type OutputResourceResponse struct {
	LocalID  string
	Provider string
	Identity any
	Name     string // Name of the underlying resource within its platform (Azure/AWS/Kubernetes)
}

type RPResourceSet struct {
	Resources []RPResource
}

func DeleteRPResource(ctx context.Context, t *testing.T, cli *radcli.CLI, client clients.ApplicationsManagementClient, resource RPResource) error {
	if resource.Type == EnvironmentsResource {
		t.Logf("deleting environment: %s", resource.Name)

		var respFromCtx *http.Response
		ctxWithResp := runtime.WithCaptureResponse(ctx, &respFromCtx)

		_, err := client.DeleteEnv(ctxWithResp, resource.Name)

		if respFromCtx.StatusCode == 204 {
			output.LogInfo("Environment '%s' does not exist or has already been deleted.", resource.Name)
		}

		return err

		// TODO: this should probably call the CLI, but if you create an
		// environment via bicep deployment, it will not be reflected in the
		// rad config.
		// return cli.EnvDelete(ctx, resource.Name)
	} else if resource.Type == ApplicationsResource {
		t.Logf("deleting application: %s", resource.Name)
		return cli.ApplicationDelete(ctx, resource.Name)
	}

	return nil
}

func ValidateRPResources(ctx context.Context, t *testing.T, expected *RPResourceSet, client clients.ApplicationsManagementClient) {
	for _, expectedResource := range expected.Resources {
		if expectedResource.Type == EnvironmentsResource {
			envs, err := client.ListEnvironmentsInResourceGroup(ctx)
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
			res, err := client.ShowResource(ctx, expectedResource.Type, expectedResource.Name)
			require.NoError(t, err)
			require.NotNil(t, res, "resource %s with type %s does not exist", expectedResource.Name, expectedResource.Type)

			if expectedResource.App != "" {
				require.True(t, strings.HasSuffix(res.Properties["application"].(string), expectedResource.App), "resource %s with type %s was not found in application %s", expectedResource.Name, expectedResource.Type, expectedResource.App)
			}

			// Validate expected output resources are present in the response
			if len(expectedResource.OutputResources) > 0 {
				bytes, err := json.Marshal(res.Properties["status"])
				require.NoError(t, err)

				var outputResourcesMap map[string][]OutputResourceResponse
				err = json.Unmarshal(bytes, &outputResourcesMap)
				require.NoError(t, err)
				outputResources := outputResourcesMap["outputResources"]
				require.Equalf(t, len(expectedResource.OutputResources), len(outputResources), "expected output resources: %v, actual: %v", expectedResource.OutputResources, outputResources)
				for _, expectedOutputResource := range expectedResource.OutputResources {
					found := false
					for _, actualOutputResource := range outputResources {
						if expectedOutputResource.LocalID == actualOutputResource.LocalID && expectedOutputResource.Provider == actualOutputResource.Provider {
							found = true
							// if the test has the OutputResourceResponse.Name set then validate
							// the actual outputresource name with the expected resource name based on the provider type.
							if expectedOutputResource.Name != "" {
								if expectedOutputResource.Provider == resourcemodel.ProviderAzure {
									identity := actualOutputResource.Identity.(map[string]interface{})
									actualID := identity["id"].(string)
									actualResource, err := resources.ParseResource(actualID)
									require.NoError(t, err)
									require.Equal(t, expectedOutputResource.Name, actualResource.Name())
								}
								if expectedOutputResource.Provider == resourcemodel.ProviderKubernetes {
									identity := actualOutputResource.Identity.(map[string]interface{})
									require.Equal(t, expectedOutputResource.Name, identity["name"].(string))
								}
							}
							break
						}
					}

					require.Truef(t, found, "expected output resource %v wasn't found", expectedOutputResource)
				}
			}
		}
	}
}
