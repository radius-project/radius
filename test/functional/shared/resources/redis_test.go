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
	"testing"

	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/shared"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_RedisManualProvisioning(t *testing.T) {
	template := "testdata/corerp-resources-redis-manualprovisioning.bicep"
	name := "corerp-resources-redis-mp"
	appNamespace := "default-corerp-resources-redis-mp"

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
						Name: "rds-app-ctnr-o",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "rds-ctnr-o",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "rds-rte-o",
						Type: validation.HttpRoutesResource,
						App:  name,
					},
					{
						Name: "rds-rds-o",
						Type: validation.O_RedisCachesResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "rds-app-ctnr-o"),
						validation.NewK8sPodForResource(name, "rds-ctnr-o"),
						validation.NewK8sServiceForResource(name, "rds-rte-o"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_RedisRecipe(t *testing.T) {
	template := "testdata/corerp-resources-redis-recipe.bicep"
	name := "corerp-resources-redis-recipe"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetBicepRecipeRegistry(), functional.GetBicepRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "corerp-resources-environment-recipe-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "rds-recipe-o",
						Type: validation.O_RedisCachesResource,
						App:  name,
					},
				},
			},
			SkipObjectValidation: true,
		},
	})

	test.Test(t)
}

func Test_RedisDefaultRecipe(t *testing.T) {
	template := "testdata/corerp-resources-redis-default-recipe.bicep"
	name := "corerp-resources-redis-default-recipe"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetBicepRecipeRegistry(), functional.GetBicepRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "corerp-resources-environment-default-recipe-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "rds-default-recipe-o",
						Type: validation.O_RedisCachesResource,
						App:  name,
					},
				},
			},
			SkipObjectValidation: true,
		},
	})

	test.Test(t)
}
