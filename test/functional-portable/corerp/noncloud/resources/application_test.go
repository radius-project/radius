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
	"fmt"
	"strings"
	"testing"

	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"

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
	appNamespace := name

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: "http-front-ctnr-simple1",
						Type: validation.ComputeContainersResource,
						App:  name,
					},
					{
						Name: "http-back-ctnr-simple1",
						Type: validation.ComputeContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "http-front-ctnr-simple1"),
						validation.NewK8sPodForResource(name, "http-back-ctnr-simple1"),
						validation.NewK8sServiceForResource(name, "http-front-ctnr-simple1"),
						validation.NewK8sServiceForResource(name, "http-back-ctnr-simple1"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				// Verify the application graph
				appManagementClient, ok := ct.Options.ManagementClient.(*clients.UCPApplicationsManagementClient)
				require.True(t, ok, "expected UCPApplicationsManagementClient")
				appGraphClient, err := v20250801preview.NewApplicationsClient(&aztoken.AnonymousCredential{}, appManagementClient.ClientOptions)
				require.NoError(t, err)

				res, err := appGraphClient.GetGraph(ctx, appManagementClient.RootScope, name, v20250801preview.GetGraphRequest{}, nil)
				require.NoError(t, err)

				// Validate the migrated graph shape dynamically so the assertion stays
				// portable across clusters and resource groups. The two containers must
				// be present on the new Radius.Compute/containers type, carry the
				// expected application/environment references, and the front->back
				// connection (whose source is a resource ID) must render as a graph edge
				// in both directions.
				expectedAppID := fmt.Sprintf("%s/providers/Radius.Core/applications/%s", appManagementClient.RootScope, name)
				expectedEnvID := fmt.Sprintf("%s/providers/Radius.Core/environments/%s-env", appManagementClient.RootScope, name)

				actualByName := make(map[string]*v20250801preview.ApplicationGraphResource, len(res.Resources))
				for _, r := range res.Resources {
					require.NotNil(t, r.Name)
					require.NotNil(t, r.Type)
					actualByName[*r.Name] = r

					if *r.Type != "Radius.Compute/containers" {
						continue
					}
					require.NotNil(t, r.Properties, "%s: Properties should not be nil", *r.Name)
					// UCP normalizes the resourceGroups segment casing, so compare IDs case-insensitively.
					require.Truef(t, strings.EqualFold(expectedAppID, fmt.Sprintf("%v", r.Properties["application"])),
						"%s: application property mismatch: expected %q got %q", *r.Name, expectedAppID, r.Properties["application"])
					require.Truef(t, strings.EqualFold(expectedEnvID, fmt.Sprintf("%v", r.Properties["environment"])),
						"%s: environment property mismatch: expected %q got %q", *r.Name, expectedEnvID, r.Properties["environment"])
					containers, ok := r.Properties["containers"].(map[string]any)
					require.Truef(t, ok, "%s: containers property should be an object", *r.Name)
					require.Containsf(t, containers, *r.Name, "%s: missing named container property", *r.Name)
				}

				front := actualByName["http-front-ctnr-simple1"]
				require.NotNil(t, front, "front container should be present in graph")
				require.Equal(t, "Radius.Compute/containers", *front.Type)
				back := actualByName["http-back-ctnr-simple1"]
				require.NotNil(t, back, "back container should be present in graph")
				require.Equal(t, "Radius.Compute/containers", *back.Type)

				require.Len(t, front.Connections, 1, "front container should have one outbound connection")
				require.NotNil(t, front.Connections[0].Direction)
				require.NotNil(t, front.Connections[0].ID)
				require.NotNil(t, back.ID)
				require.Equal(t, v20250801preview.DirectionOutbound, *front.Connections[0].Direction)
				require.Equal(t, *back.ID, *front.Connections[0].ID)

				require.Len(t, back.Connections, 1, "back container should have one inbound connection")
				require.NotNil(t, back.Connections[0].Direction)
				require.NotNil(t, back.Connections[0].ID)
				require.NotNil(t, front.ID)
				require.Equal(t, v20250801preview.DirectionInbound, *back.Connections[0].Direction)
				require.Equal(t, *front.ID, *back.Connections[0].ID)
			},
		},
	})

	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, appNamespace)
	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployExecutor(template, testutil.GetMagpieImage(), fmt.Sprintf("environment=%s", previewEnvID))

	test.Test(t)
}
