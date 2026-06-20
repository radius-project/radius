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

package resource_test

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"

	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/stretchr/testify/require"
)

func Test_Application(t *testing.T) {
	template := "testdata/corerp-resources-application.bicep"
	name := "corerp-resources-application"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "corerp-resources-application-app",
						Type: validation.CoreApplicationsResource,
					},
				},
			},
			// Application should not render any K8s Objects directly
			K8sObjects: &validation.K8sObjectSet{},
		},
	})
	test.Test(t)
}

func Test_ApplicationGraph(t *testing.T) {
	// Deploy a simple app
	template := "testdata/corerp-resources-application-graph.bicep"
	name := "corerp-application-simple1"
	appNamespace := "default-corerp-application-simple1"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetMagpieImage()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "http-front-ctnr-simple1",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "http-back-ctnr-simple1",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "http-front-ctnr-simple1"),
						validation.NewK8sPodForResource(name, "http-back-ctnr-simple1"),
						validation.NewK8sServiceForResource(name, "http-front-cntr-simple1").ValidateLabels(false),
						validation.NewK8sServiceForResource(name, "http-back-cntr-simple1").ValidateLabels(false),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				// Verify the application graph
				options := rp.NewRPTestOptions(t)
				client := options.ManagementClient
				require.IsType(t, client, &clients.UCPApplicationsManagementClient{})

				appManagementClient := client.(*clients.UCPApplicationsManagementClient)
				appGraphClient, err := v20231001preview.NewApplicationsClient(&aztoken.AnonymousCredential{}, appManagementClient.ClientOptions)
				require.NoError(t, err)

				res, err := appGraphClient.GetGraph(ctx, appManagementClient.RootScope, "corerp-application-simple1", v20231001preview.GetGraphRequest{}, nil)
				require.NoError(t, err)

				// assert that the graph is as expected
				expected := []*v20231001preview.ApplicationGraphResource{}
				testutil.MustUnmarshalFromFile("corerp-resources-application-graph-out.json", &expected)

				// For easier comparison, we sort the resources by name.
				sort.Slice(res.Resources, func(i, j int) bool {
					return *res.Resources[i].Name < *res.Resources[j].Name
				})
				sort.Slice(expected, func(i, j int) bool {
					return *expected[i].Name < *expected[j].Name
				})

				// Verify the resource-type-specific Properties bag. We assert
				// the top-level keys and compare the value of `application`
				// (a stable, deterministic string).
				//
				// We build a name -> *expected lookup so the Properties graft
				// below is robust to differences in ordering or length between
				// `res.Resources` and the fixture (otherwise an index-based
				// graft would silently attach properties to the wrong element
				// when ElementsMatch is used as the fallback assertion).
				expectedByName := make(map[string]*v20231001preview.ApplicationGraphResource, len(expected))
				for i := range expected {
					expectedByName[*expected[i].Name] = expected[i]
				}

				for _, r := range res.Resources {
					if *r.Type != "Applications.Core/containers" {
						continue
					}
					require.NotNil(t, r.Properties, "%s: Properties should not be nil", *r.Name)

					// The application that owns this container lives in the
					// same resource group; derive its expected ID from the
					// container's own ID so the assertion is independent of
					// the resource group name used by the test environment.
					expectedAppID := strings.Replace(
						*r.ID,
						"/containers/"+*r.Name,
						"/applications/"+name,
						1,
					)
					require.Equal(t, expectedAppID, r.Properties["application"], "%s: application property mismatch", *r.Name)
					require.Contains(t, r.Properties, "container", "%s: missing container property", *r.Name)

					// Graft the actual Properties onto the matching expected
					// resource so the struct-level equality check below
					// succeeds for the rest of the fields without having to
					// encode the environment-specific property bag in the
					// fixture.
					if e, ok := expectedByName[*r.Name]; ok {
						e.Properties = r.Properties
					}
				}

				if len(res.Resources) != len(expected) {
					require.ElementsMatch(t, expected, res.Resources)
				} else {
					for i := range res.Resources {
						require.Equal(t, expected[i], res.Resources[i], *expected[i].Name)
					}
				}
			},
		},
	})

	test.Test(t)
}
