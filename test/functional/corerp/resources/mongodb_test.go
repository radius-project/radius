// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"os"
	"testing"

	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_MongoDB(t *testing.T) {
	template := "testdata/corerp-resources-mongodb.bicep"
	name := "corerp-resources-mongodb"

	if os.Getenv("AZURE_MONGODB_RESOURCE_ID") == "" {
		t.Error("AZURE_MONGODB_RESOURCE_ID environment variable must be set to run this test.")
	}
	mongodbresourceid := "mongodbresourceid=" + os.Getenv("AZURE_MONGODB_RESOURCE_ID")
	appNamespace := "default-corerp-resources-mongodb"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), mongodbresourceid),
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
	// template using recipe testdata/recipes/test-recipes/mongodb-recipe-kubernetes.bicep
	template := "testdata/corerp-resources-mongodb-recipe.bicep"
	name := "corerp-resources-mongodb-recipe"
	appNamespace := "corerp-resources-mongodb-recipe-app"
	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), functional.GetRecipeRegistry(), functional.GetRecipeVersion()),
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
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "mongodb-recipe-app-ctnr").ValidateLabels(false),
						validation.NewK8sPodForResource(name, "mongo-recipe-resource").ValidateLabels(false),
					},
				},
			},
		},
	})

	test.Test(t)
}

// Test_MongoDB_Recipe_Parameters validates the creation of a mongoDB from recipe with parameters passed by operator while linking recipe
// and developer while creating the mongoDatabase link.
// If the same parameters are set by the developer and the operator then the developer parameters are applied in to resolve conflicts.
// Container uses the mongoDB link to connect to the mongoDB resource
func Test_MongoDB_Recipe_Parameters(t *testing.T) {
	template := "testdata/corerp-resources-mongodb-recipe-parameters.bicep"
	name := "corerp-resources-mongodb-recipe-parameters"
	appNamespace := "corerp-resources-mongodb-recipe-param-app"
	rg := os.Getenv("INTEGRATION_TEST_RESOURCE_GROUP_NAME")
	// Error the test if INTEGRATION_TEST_RESOURCE_GROUP_NAME is not set
	// for running locally set the INTEGRATION_TEST_RESOURCE_GROUP_NAME with the test resourceGroup
	if rg == "" {
		t.Error("This test needs the env variable INTEGRATION_TEST_RESOURCE_GROUP_NAME to be set")
	}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), functional.GetRecipeRegistry(), functional.GetRecipeVersion()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-environment-recipe-parameters-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: name,
						Type: validation.ApplicationsResource,
						App:  name,
					},
					{
						Name: "mdb-param-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "mdb-recipe-param-db",
						Type: validation.MongoDatabasesResource,
						App:  name,
						OutputResources: []validation.OutputResourceResponse{
							{
								Provider: resourcemodel.ProviderAzure,
								LocalID:  rpv1.LocalIDAzureCosmosAccount,
								Name:     "acnt-developer-" + rg,
							},
							{
								Provider: resourcemodel.ProviderAzure,
								LocalID:  rpv1.LocalIDAzureCosmosDBMongo,
								Name:     "mdb-operator-" + rg,
							},
						},
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "mdb-param-ctnr"),
					},
				},
			},
		},
	})

	test.Test(t)
}

// Test_MongoDB_Recipe_ContextParameter validates creation of a mongoDB from
// recipe using the context parameter generated and set by linkRP,
// and container using the mongoDB link to connect to the underlying mongoDB resource.
func Test_MongoDB_Recipe_ContextParameter(t *testing.T) {
	template := "testdata/corerp-resources-mongodb-recipe-context.bicep"
	name := "corerp-resources-mongodb-recipe-context"
	appNamespace := "corerp-resources-mongodb-recipe-context-app"
	rg := os.Getenv("INTEGRATION_TEST_RESOURCE_GROUP_NAME")
	// Error the test if INTEGRATION_TEST_RESOURCE_GROUP_NAME is not set
	// for running locally set the INTEGRATION_TEST_RESOURCE_GROUP_NAME with the test resourceGroup
	if rg == "" {
		t.Error("This test needs the env variable INTEGRATION_TEST_RESOURCE_GROUP_NAME to be set")
	}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), functional.GetRecipeRegistry(), functional.GetRecipeVersion()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-environment-recipes-context-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: name,
						Type: validation.ApplicationsResource,
						App:  name,
					},
					{
						Name: "mdb-ctx-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "mdb-ctx",
						Type: validation.MongoDatabasesResource,
						App:  name,
						OutputResources: []validation.OutputResourceResponse{
							{
								Provider: resourcemodel.ProviderAzure,
								LocalID:  rpv1.LocalIDAzureCosmosAccount,
							},
							{
								Provider: resourcemodel.ProviderAzure,
								LocalID:  rpv1.LocalIDAzureCosmosDBMongo,
								Name:     "mdb-ctx-" + rg,
							},
						},
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "mdb-ctx-ctnr"),
					},
				},
			},
		},
	})

	test.Test(t)
}
