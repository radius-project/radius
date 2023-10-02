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
	name := "dsrp-resources-simulatedenv-recipe"
	appNamespace := "dsrp-resources-simulatedenv-recipe-app"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), functional.GetBicepRecipeRegistry(), functional.GetBicepRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "dsrp-resources-simenv-recipe-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: "dsrp-resources-simenv-recipe",
						Type: validation.ApplicationsResource,
						App:  name,
					},
					{
						Name: "mongodb-app-ctnr-simenv",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "mongodb-db-simenv",
						Type: validation.MongoDatabasesResource,
						App:  name,
					},
				},
			},
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, ct shared.RPTest) {
				// Get pods in app namespace
				label := fmt.Sprintf("radius.dev/application=%s", name)
				pods, err := ct.Options.K8sClient.CoreV1().Pods(appNamespace).List(ctx, metav1.ListOptions{
					LabelSelector: label,
				})
				require.NoError(t, err)
				// Verify no actual pods are deployed
				require.Equal(t, 0, len(pods.Items))
			},
		},
	})

	test.Test(t)
}
