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

	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
)

func Test_Redis_Recipe(t *testing.T) {
	template := "testdata/datastoresrp-resources-redis-recipe.bicep"
	name := "dsrp-resources-redis-recipe"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetBicepRecipeRegistry(), testutil.GetBicepRecipeVersion()),
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
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				redis, err := test.Options.ManagementClient.GetResource(ctx, "Applications.Datastores/redisCaches", "rds-recipe")
				require.NoError(t, err)
				require.NotNil(t, redis)
				status := redis.Properties["status"].(map[string]any)
				recipe := status["recipe"].(map[string]interface{})
				require.Equal(t, "bicep", recipe["templateKind"].(string))
				templatePath := strings.Split(recipe["templatePath"].(string), ":")[0]
				require.Equal(t, "ghcr.io/radius-project/dev/test/testrecipes/test-bicep-recipes/redis-recipe-value-backed", templatePath)
			},
		},
	})

	test.Test(t)
}

func Test_Redis_DefaultRecipe(t *testing.T) {
	template := "testdata/datastoresrp-resources-redis-default-recipe.bicep"
	name := "dsrp-resources-redis-default-recipe"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetBicepRecipeRegistry(), testutil.GetBicepRecipeVersion()),
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
