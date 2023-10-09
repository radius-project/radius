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
	"testing"

	"github.com/radius-project/radius/test/functional"
	"github.com/radius-project/radius/test/functional/shared"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_Deployment_SimulatedEnv_BicepRecipe(t *testing.T) {
	template := "testdata/datastoresrp-resources-simulatedenv-recipe.bicep"
	appName := "dsrp-resources-simenv-recipe"
	containerName := "mongodb-app-ctnr-simenv"
	appNamespace := "dsrp-resources-simulatedenv-recipe-app"
	mongoDBName := "mongodb-db-simenv"
	envName := "dsrp-resources-simenv-recipe-env"

	test := shared.NewRPTest(t, appName, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), functional.GetBicepRecipeRegistry(), functional.GetBicepRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: envName,
						Type: validation.EnvironmentsResource,
					},
					{
						Name: appName,
						Type: validation.ApplicationsResource,
						App:  appName,
					},
					{
						Name: containerName,
						Type: validation.ContainersResource,
						App:  appName,
					},
					{
						Name: mongoDBName,
						Type: validation.MongoDatabasesResource,
						App:  appName,
					},
				},
			},
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, ct shared.RPTest) {
				// Get pods in app namespace
				label := fmt.Sprintf("radius.dev/application=%s", appName)
				pods, err := ct.Options.K8sClient.CoreV1().Pods(appNamespace).List(ctx, metav1.ListOptions{
					LabelSelector: label,
				})
				require.NoError(t, err)
				// Verify no actual pods are deployed
				require.Equal(t, 0, len(pods.Items))

				env, err := ct.Options.ManagementClient.GetEnvDetails(ctx, envName)
				require.NoError(t, err)
				require.True(t, *env.Properties.Simulated)

				resources, err := ct.Options.ManagementClient.ListAllResourcesByApplication(ctx, appName)
				require.NoError(t, err)
				require.Equal(t, 2, len(resources))
				require.Equal(t, mongoDBName, *resources[0].Name)
				require.Equal(t, "Applications.Datastores/mongoDatabases", *resources[0].Type)
				require.Equal(t, containerName, *resources[1].Name)
				require.Equal(t, "Applications.Core/containers", *resources[1].Type)
			},
		},
	})

	test.Test(t)
}
