// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package validation

import (
	"context"
	"testing"

	"github.com/project-radius/radius/pkg/cli/clients"
)

const (
	EnvironmentsResource = "environments"
	ApplicationsResource = "applications"
	HttpRoutesResource   = "httpRoutes"
	ContainersResource   = "containers"
)

type Resource struct {
	Type string
	Name string
}

func ValidateCoreRPResources(ctx context.Context, t *testing.T, expected []Resource, client clients.ApplicationsManagementClient) {
	// Pending: https://github.com/project-radius/radius/issues/2726
	// for _, resource := range expected {
	// 	if resource.Type == EnvironmentsResource {
	// 		env, err := client.GetEnvDetails(ctx, resource.Name)
	// 		require.NoError(t, err)
	// 		fmt.Println(env)
	// 	} else if resource.Type == ApplicationsResource {
	// 		apps, err := client.ListApplications(ctx)
	// 		require.NoError(t, err)
	// 		require.NotEmpty(t, apps)

	// 		found := false
	// 		for _, app := range apps {
	// 			if *app.Name == resource.Name {
	// 				found = true
	// 				break
	// 			}
	// 		}
	// 		require.True(t, found, fmt.Sprintf("application %s was not found", resource.Name))
	// 	} else {

	// 	}
	// }

}
