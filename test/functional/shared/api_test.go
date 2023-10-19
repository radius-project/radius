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
	"context"
	"fmt"
	"testing"

	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_radius "github.com/radius-project/radius/pkg/ucp/resources/radius"
	"github.com/radius-project/radius/test/functional"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/radius-project/radius/test/validation"
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

// Test_ApplicationGraph covers the application graph API.

// This test exists as a smoke test that this API can be called safely.

func Test_ApplicationGraph(t *testing.T) {
	// Deploy a simple app
	template := "testdata/corerp-resources-application-graph.bicep"
	name := "corerp-application-simple"
	appNamespace := "corerp-application-simple"

	test := NewRPTest(t, name, []TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "http-front-ctnr-simple",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "http-back-rte-simple",
						Type: validation.HttpRoutesResource,
						App:  name,
					},
					{
						Name: "http-back-ctnr-simple",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "http-front-ctnr-simple"),
						validation.NewK8sPodForResource(name, "http-back-ctnr-simple"),
						validation.NewK8sServiceForResource(name, "http-back-rte-simple"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct RPTest) {
				// Verify the application graph
				options := NewRPTestOptions(t)
				client := options.ManagementClient
				require.IsType(t, client, &clients.UCPApplicationsManagementClient{})
				appManagementClient := client.(*clients.UCPApplicationsManagementClient)
				appGraphClient, err := v20231001preview.NewApplicationsClient(appManagementClient.RootScope, &aztoken.AnonymousCredential{}, appManagementClient.ClientOptions)
				_, err = appGraphClient.GetGraph(ctx, "corerp-application-simple", map[string]any{}, nil)
				//require(res, )
				require.NoError(t, err)

			},
		},
	})

	test.Test(t)

	// getGraph on the app and verify it's what we expect
}
