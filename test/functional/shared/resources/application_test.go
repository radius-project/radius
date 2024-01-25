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
	"testing"

	"github.com/radius-project/radius/test/functional"
	"github.com/radius-project/radius/test/functional/shared"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"

	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_Application(t *testing.T) {
	template := "testdata/corerp-resources-application.bicep"
	name := "corerp-resources-application"
	appNamespace := "corerp-resources-application-app"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "corerp-resources-application-app",
						Type: validation.ApplicationsResource,
					},
				},
			},
			// Application should not render any K8s Objects directly
			K8sObjects: &validation.K8sObjectSet{},
			PostStepVerify: func(ctx context.Context, t *testing.T, test shared.RPTest) {
				_, err := test.Options.K8sClient.CoreV1().Namespaces().Get(ctx, appNamespace, metav1.GetOptions{})
				require.NoErrorf(t, err, "%s must be created", appNamespace)
			},
		},
	})
	test.Test(t)
}

func Test_ApplicationGraph(t *testing.T) {
	// Deploy a simple app
	template := "testdata/corerp-resources-application-graph.bicep"
	name := "corerp-application-simple1"
	appNamespace := "default-corerp-application-simple1"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
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
						Name: "http-back-rte-simple1",
						Type: validation.HttpRoutesResource,
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
						validation.NewK8sServiceForResource(name, "http-back-rte-simple1"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct shared.RPTest) {
				// Verify the application graph
				options := shared.NewRPTestOptions(t)
				client := options.ManagementClient
				require.IsType(t, client, &clients.UCPApplicationsManagementClient{})

				appManagementClient := client.(*clients.UCPApplicationsManagementClient)
				appGraphClient, err := v20231001preview.NewApplicationsClient(appManagementClient.RootScope, &aztoken.AnonymousCredential{}, appManagementClient.ClientOptions)
				require.NoError(t, err)

				res, err := appGraphClient.GetGraph(ctx, "corerp-application-simple1", map[string]any{}, nil)
				require.NoError(t, err)

				// assert that the graph is as expected
				expected := []*v20231001preview.ApplicationGraphResource{}
				testutil.MustUnmarshalFromFile("corerp-resources-application-graph-out.json", &expected)
				require.ElementsMatch(t, expected, res.Resources)
			},
		},
	})

	test.Test(t)
}
