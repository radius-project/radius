// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package validation

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/output"
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

	MongoDatabasesResource        = "applications.link/mongoDatabases"
	RabbitMQMessageQueuesResource = "applications.link/rabbitMQMessageQueues"
	RedisCachesResource           = "applications.link/redisCaches"
	SQLDatabasesResource          = "applications.link/sqlDatabases"
	DaprPubSubBrokersResource     = "applications.link/daprPubSubBrokers"
	DaprSecretStoresResource      = "applications.link/daprSecretStores"
	DaprStateStoresResource       = "applications.link/daprStateStores"
	DaprInvokeHttpRoutesResource  = "applications.link/daprInvokeHttpRoutes"
	ExtendersResource             = "applications.link/extenders"
)

type CoreRPResource struct {
	Type string
	Name string
	App  string
}

type CoreRPResourceSet struct {
	Resources []CoreRPResource
}

func DeleteCoreRPResource(ctx context.Context, t *testing.T, cli *radcli.CLI, client clients.ApplicationsManagementClient, resource CoreRPResource) error {
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

func ValidateCoreRPResources(ctx context.Context, t *testing.T, expected *CoreRPResourceSet, client clients.ApplicationsManagementClient) {
	for _, resource := range expected.Resources {
		if resource.Type == EnvironmentsResource {
			envs, err := client.ListEnvironmentsInResourceGroup(ctx)
			require.NoError(t, err)
			require.NotEmpty(t, envs)

			found := false
			for _, env := range envs {
				if *env.Name == resource.Name {
					found = true
					continue
				}
			}

			require.True(t, found, fmt.Sprintf("environment %s was not found", resource.Name))
		} else if resource.Type == ApplicationsResource {
			apps, err := client.ListApplications(ctx)
			require.NoError(t, err)
			require.NotEmpty(t, apps)

			found := false
			for _, app := range apps {
				if *app.Name == resource.Name {
					found = true
					continue
				}
			}

			require.True(t, found, fmt.Sprintf("application %s was not found", resource.Name))
		} else {
			res, err := client.ShowResource(ctx, resource.Type, resource.Name)
			require.NoError(t, err)
			require.NotNil(t, res, "resource %s with type %s does not exist", resource.Name, resource.Type)

			if resource.App != "" {
				require.True(t, strings.HasSuffix(res.Properties["application"].(string), resource.App), "resource %s with type %s was not found in application %s", resource.Name, resource.Type, resource.App)
			}
		}
	}
}
