// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package validation

import (
	"context"
	"fmt"
	"testing"

	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/stretchr/testify/require"

	"github.com/project-radius/radius/test/radcli"
)

const (
	EnvironmentsResource = "applications.core/environments"
	ApplicationsResource = "applications.core/applications"
	HttpRoutesResource   = "applications.core/httpRoutes"
	GatewaysResource     = "applications.core/gateways"
	ContainersResource   = "applications.core/containers"

	MongoDatabasesResource        = "applications.connector/mongoDatabases"
	RabbitMQMessageQueuesResource = "applications.connector/rabbitMQMessageQueues"
	RedisCachesResource           = "applications.connector/redisCaches"
	SQLDatabasesResource          = "applications.connector/sqlDatabases"
	DaprPubSubResource            = "applications.connector/daprPubSubBrokers"
	DaprSecretStoreResource       = "applications.connector/daprSecretStores"
	DaprStateStoreResource        = "applications.connector/daprStateStores"
	DaprInvokeHttpRoute           = "applications.connector/daprInvokeHttpRoutes"
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
		return client.DeleteEnv(ctx, resource.Name)

		// TODO: this should probably call the CLI, but if you create an
		// environment via bicep deployment, it will not be reflected in the
		// rad config.
		// return cli.EnvDelete(ctx, resource.Name)
	} else if resource.Type == ApplicationsResource {
		t.Logf("deleting application: %s", resource.Name)
		return cli.ApplicationDelete(ctx, resource.Name)
	}

	t.Logf("resource %s is not an application or an environment. skipping...", resource.Name)
	return nil
}

func ValidateCoreRPResources(ctx context.Context, t *testing.T, expected *CoreRPResourceSet, client clients.ApplicationsManagementClient) {
	for _, resource := range expected.Resources {
		if resource.Type == EnvironmentsResource {
			envs, err := client.ListEnv(ctx)
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
			require.NotEmpty(t, resource.App)
			appResources, err := client.ListAllResourceOfTypeInApplication(ctx, resource.App, resource.Type)
			require.NoError(t, err)
			require.NotEmpty(t, appResources)
			found := false
			for _, appResource := range appResources {
				if *appResource.Name == resource.Name {
					found = true
					continue
				}
			}

			require.True(t, found, fmt.Sprintf("resource %s with type %s was not found in application %s", resource.Name, resource.Type, resource.App))
		}
	}
}
