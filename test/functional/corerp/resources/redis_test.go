// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"os"
	"testing"

	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_Redis(t *testing.T) {
	template := "testdata/corerp-resources-redis-user-secrets.bicep"
	name := "corerp-resources-redis-user-secrets"
	appNamespace := "default-corerp-resources-redis-user-secrets"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
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
						Name: "rds-rte",
						Type: validation.HttpRoutesResource,
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
						validation.NewK8sServiceForResource(name, "rds-rte"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_RedisAzure(t *testing.T) {
	template := "testdata/corerp-resources-redis-azure.bicep"
	name := "corerp-resources-redis-azure"

	if os.Getenv("REDIS_RESOURCE_ID") == "" {
		t.Error("failed to get the redis resource id from the environment")
	}
	redisresourceid := "redisresourceid=" + os.Getenv("REDIS_RESOURCE_ID")

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), redisresourceid),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "redis-azure-app-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "redis-link",
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

func Test_RedisValueBackedRecipe(t *testing.T) {
	template := "testdata/corerp-resources-redis-value-backed-recipe.bicep"
	name := "corerp-resources-redis-value-backed-recipe"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-environment-value-backed-recipe-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "rds-value-backed-recipe",
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
