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

package shared

import (
	"fmt"
	"testing"

	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_radius "github.com/radius-project/radius/pkg/ucp/resources/radius"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"

	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
)

// Test_ResourceList covers the plane and resource-group scope list APIs for all Radius resource types.
//
// This test exists as a smoke test that these APIs can be called safely. They are mainly used by the CLI
// at this time, and this is a better way to get coverage.
func Test_ResourceList(t *testing.T) {
	options := NewRPTestOptions(t)

	// Extract the scope and client options from the management client so we can make our own API calls.
	require.IsType(t, options.ManagementClient, &clients.UCPApplicationsManagementClient{})
	scope := options.ManagementClient.(*clients.UCPApplicationsManagementClient).RootScope
	clientOptions := options.ManagementClient.(*clients.UCPApplicationsManagementClient).ClientOptions

	parsed, err := resources.ParseScope("/" + scope)
	require.NoError(t, err)
	require.NotEmpty(t, parsed.FindScope(resources_radius.ScopeResourceGroups), "workspace scope must contain resource group segment")

	resourceGroupScope := parsed.String()
	planeScope := parsed.Truncate().String()

	resourceTypes := []string{"Applications.Core/applications", "Applications.Core/environments"}
	resourceTypes = append(resourceTypes, clients.ResourceTypesList...)

	listResources := func(t *testing.T, rootScope string, resourceType string) {
		ctx, cancel := testcontext.NewWithCancel(t)
		t.Cleanup(cancel)
		client, err := generated.NewGenericResourcesClient(resourceGroupScope, resourceType, &aztoken.AnonymousCredential{}, clientOptions)
		require.NoError(t, err)

		pager := client.NewListByRootScopePager(nil)
		for pager.More() {
			nextPage, err := pager.NextPage(ctx)
			require.NoError(t, err)

			// We're just logging what we find in case there are issues to troubleshoot.
			// Other tests will be creating, updating, and deleting resources while these are running, so
			// we can't really assert anything about the results.
			//
			// This test exists just to make sure the APIs can be called safely.
			for _, resource := range nextPage.GenericResourcesList.Value {
				t.Log("found resource: " + *resource.ID)
			}
		}
	}

	for _, resourceType := range resourceTypes {
		resourceType := resourceType // capture range variable
		t.Run(fmt.Sprintf("list at resource-group scope: %s", resourceType), func(t *testing.T) {
			t.Parallel()
			listResources(t, resourceGroupScope, resourceType)
		})

		t.Run(fmt.Sprintf("list at plane scope: %s", resourceType), func(t *testing.T) {
			t.Parallel()
			listResources(t, planeScope, resourceType)
		})
	}
}
