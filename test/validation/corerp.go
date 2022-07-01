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
)

const (
	EnvironmentsResource   = "applications.core/environments"
	ApplicationsResource   = "applications.core/applications"
	HttpRoutesResource     = "applications.core/httpRoutes"
	MongoDatabasesResource = "applications.core/mongoDatabases"
	RedisCachesResource    = "applications.core/redisCaches"
	ContainersResource     = "applications.core/containers"
)

type Resource struct {
	Type string
	Name string
}

func ValidateCoreRPResources(ctx context.Context, t *testing.T, expected []Resource, client clients.ApplicationsManagementClient) {
	for _, resource := range expected {
		if resource.Type == EnvironmentsResource {
			envs, err := client.ListEnv(ctx)
			require.NoError(t, err)
			require.NotEmpty(t, envs)

			found := false
			for _, app := range envs {
				if *app.Name == resource.Name {
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
			require.Fail(t, "unhandled resource type")
		}
	}
}
