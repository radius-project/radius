// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"testing"

	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_MongoDB(t *testing.T) {
	t.Skip()
	template := "testdata/corerp-resources-mongodb.bicep"
	name := "corerp-resources-mongodb"
	appNamespace := "default-corerp-resources-mongodb"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-mongodb",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "mdb-app-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "mdb-db",
						Type: validation.MongoDatabasesResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "mdb-app-ctnr"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_MongoDBUserSecrets(t *testing.T) {
	template := "testdata/corerp-resources-mongodb-user-secrets.bicep"
	name := "corerp-resources-mongodb-user-secrets"
	appNamespace := "default-corerp-resources-mongodb-user-secrets"

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
						Name: "mdb-us-app-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "mdb-us-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "mdb-us-rte",
						Type: validation.HttpRoutesResource,
						App:  name,
					},
					{
						Name: "mdb-us-db",
						Type: validation.MongoDatabasesResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "mdb-us-app-ctnr"),
						validation.NewK8sPodForResource(name, "mdb-us-ctnr"),
						validation.NewK8sServiceForResource(name, "mdb-us-rte"),
					},
				},
			},
		},
	})

	test.Test(t)
}

// Test_MongoDB_Recipe validates:
// the creation of a mongoDB from recipe
// container using the mongoDB link to connect to the mongoDB resource
func Test_MongoDB_Recipe(t *testing.T) {
	template := "testdata/corerp-resources-mongodb-recipe.bicep"
	name := "corerp-resources-mongodb-recipe"
	appNamespace := "corerp-resources-mongodb-recipe-app"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
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
						OutputResources: []validation.OutputResourceResponse{
							{
								Provider: resourcemodel.ProviderAzure,
								LocalID:  outputresource.LocalIDAzureCosmosAccount,
							},
							{
								Provider: resourcemodel.ProviderAzure,
								LocalID:  outputresource.LocalIDAzureCosmosDBMongo,
							},
						},
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "mongodb-recipe-app-ctnr"),
					},
				},
			},
		},
	})

	test.Test(t)
}

// Test_MongoDB_Recipe validates:
// the creation of a mongoDB from a devrecipe that is linked to the environment when created with useDevRecipes = true
// the container using the mongoDB link to connect to the mongoDB resource
func Test_MongoDB_DevRecipe(t *testing.T) {

	template := "testdata/corerp-resources-mongodb-devrecipe.bicep"
	name := "corerp-resources-mongodb-devrecipe"
	appNamespace := "corerp-resources-mongodb-devrecipe-app"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-environment-devrecipe-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: "corerp-resources-mongodb-devrecipe",
						Type: validation.ApplicationsResource,
						App:  name,
					},
					{
						Name: "mongodb-devrecipe-app-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "mongo-devrecipe-db",
						Type: validation.MongoDatabasesResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "mongodb-devrecipe-app-ctnr"),
					},
				},
			},
		},
	})

	test.Test(t)
}

// Test_MongoDB_Recipe_Parameters validates:
// the creation of a mongoDB from recipe with parameters passed by operator while linking recipe
// and developer while creating the mongoDatabase link. The developer parameters are applied in to resolve conflicts.
// Container uses the mongoDB link to connect to the mongoDB resource
func Test_MongoDB_Recipe_Parameters(t *testing.T) {
	template := "testdata/corerp-resources-mongodb-recipe-parameters.bicep"
	name := "corerp-resources-mongodb-recipe-parameters"
	appNamespace := "corerp-resources-mongodb-recipe-param-app"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-environment-recipe-parameters-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: "corerp-resources-mongodb-recipe-parameters",
						Type: validation.ApplicationsResource,
						App:  name,
					},
					{
						Name: "mep-app-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "mongo-recipe-param-db",
						Type: validation.MongoDatabasesResource,
						App:  name,
						OutputResources: []validation.OutputResourceResponse{
							{
								Provider: resourcemodel.ProviderAzure,
								LocalID:  outputresource.LocalIDAzureCosmosAccount,
								Identity: "account-developer-parameters",
							},
							{
								Provider: resourcemodel.ProviderAzure,
								LocalID:  outputresource.LocalIDAzureCosmosDBMongo,
								Identity: "mongodb-operator-parameters",
							},
						},
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "mep-app-ctnr"),
					},
				},
			},
		},
	})

	test.VerifyRecipeResource = true
	test.Test(t)
}
