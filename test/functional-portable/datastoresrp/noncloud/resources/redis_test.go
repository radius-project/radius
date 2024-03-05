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
	"strings"
	"testing"

	"github.com/radius-project/radius/test/functional"
	"github.com/radius-project/radius/test/functional/shared"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
)

func Test_Redis_Manual(t *testing.T) {
	template := "testdata/datastoresrp-resources-redis-manual.bicep"
	name := "dsrp-resources-redis-manual"
	appNamespace := "default-dsrp-resources-redis-manual"

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
						Name: "rds-app-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "rds-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "rds-rds",
						Type: validation.RedisCachesResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "rds-app-ctnr"),
						validation.NewK8sPodForResource(name, "rds-ctnr"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_Redis_Recipe(t *testing.T) {
	template := "testdata/datastoresrp-resources-redis-recipe.bicep"
	name := "dsrp-resources-redis-recipe"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetBicepRecipeRegistry(), functional.GetBicepRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "dsrp-resources-env-recipe-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "rds-recipe",
						Type: validation.RedisCachesResource,
						App:  name,
					},
				},
			},
			SkipObjectValidation: true,
			PostStepVerify: func(ctx context.Context, t *testing.T, test shared.RPTest) {
				redis, err := test.Options.ManagementClient.ShowResource(ctx, "Applications.Datastores/redisCaches", "rds-recipe")
				require.NoError(t, err)
				require.NotNil(t, redis)
				status := redis.Properties["status"].(map[string]any)
				recipe := status["recipe"].(map[string]interface{})
				require.Equal(t, "bicep", recipe["templateKind"].(string))
				templatePath := strings.Split(recipe["templatePath"].(string), ":")[0]
				require.Equal(t, "ghcr.io/radius-project/dev/test/functional/shared/recipes/redis-recipe-value-backed", templatePath)
			},
		},
	})

	test.Test(t)
}

func Test_Redis_DefaultRecipe(t *testing.T) {
	template := "testdata/datastoresrp-resources-redis-default-recipe.bicep"
	name := "dsrp-resources-redis-default-recipe"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetBicepRecipeRegistry(), functional.GetBicepRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "dsrp-resources-env-default-recipe-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "rds-default-recipe",
						Type: validation.RedisCachesResource,
						App:  name,
					},
				},
			},
			SkipObjectValidation: true,
		},
	})

	test.Test(t)
}
