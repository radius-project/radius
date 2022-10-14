// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"testing"

	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_MongoDB_Recipe(t *testing.T) {

	template := "testdata/corerp-resources-mongodb-recipe.bicep"
	name := "corerp-resources-mongodb-recipe"

	requiredSecrets := map[string]map[string]string{}
	magpieImage := "magpieimage=pratmishra.azurecr.io/magpiego:latest"
	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, magpieImage),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-environment-recipes-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: "corerp-resources-mongodb-recipe",
						Type: validation.ApplicationsResource,
						App:  name,
					},
					{
						Name: "mongodb-recipe-app-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "mongo-recipe-db",
						Type: validation.MongoDatabasesResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"corerp-resources-environment-recipes-env": {
						validation.NewK8sPodForResource(name, "mongodb-recipe-app-ctnr"),
					},
				},
			},
		},
	}, requiredSecrets)

	test.Test(t)
}
