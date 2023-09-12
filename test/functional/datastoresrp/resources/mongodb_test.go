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
	"os"
	"testing"

	"github.com/radius-project/radius/test/functional"
	"github.com/radius-project/radius/test/functional/shared"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"
)

// Opt-out case for manual resource provisioning
func Test_MongoDB_Manual(t *testing.T) {
	template := "testdata/datastoresrp-rs-mongodb-manual.bicep"
	name := "dsrp-resources-mongodb-manual"
	appNamespace := "default-cdsrp-resources-mongodb-manual"

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
						Name: "mdb-us-db",
						Type: validation.MongoDatabasesResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "mdb-us-app-ctnr").ValidateLabels(false),
						validation.NewK8sPodForResource(name, "mdb-us-ctnr").ValidateLabels(false),
					},
				},
			},
		},
	})

	test.Test(t)
}

// Test_MongoDB_Recipe validates:
// the creation of a mongoDB from a recipe that uses an Azure resource
func Test_MongoDB_Recipe(t *testing.T) {
	template := "testdata/datastoresrp-resources-mongodb-recipe.bicep"
	name := "dsrp-resources-mongodb-recipe"
	appNamespace := "dsrp-resources-mongodb-recipe-app"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), functional.GetBicepRecipeRegistry(), functional.GetBicepRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "dsrp-resources-mongodb-recipe-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: "dsrp-resources-mongodb-recipe",
						Type: validation.ApplicationsResource,
						App:  name,
					},
					{
						Name: "mongodb-app-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "mongodb-db",
						Type: validation.MongoDatabasesResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "mongodb-app-ctnr").ValidateLabels(false),
					},
				},
			},
		},
	})

	test.Test(t)
}

// Test_MongoDB_Recipe_Parameters validates the creation of a mongoDB from recipe with parameters passed by operator while linking recipe
// and developer while creating the mongoDatabase resource.
// If the same parameters are set by the developer and the operator then the developer parameters are applied in to resolve conflicts.
// Container uses the mongoDB resource to connect to the mongoDB resource
func Test_MongoDB_RecipeParameters(t *testing.T) {
	t.Skip("Skipping test as creating/deleting cosmosdb resource is unreliable - https://github.com/radius-project/radius/issues/5929")

	template := "testdata/datastoresrp-resources-mongodb-recipe-parameters.bicep"
	name := "dsrp-resources-mongodb-recipe-parameters"
	appNamespace := "dsrp-resources-mongodb-recipe-param-app"
	rg := os.Getenv("INTEGRATION_TEST_RESOURCE_GROUP_NAME")
	// Error the test if INTEGRATION_TEST_RESOURCE_GROUP_NAME is not set
	// for running locally set the INTEGRATION_TEST_RESOURCE_GROUP_NAME with the test resourceGroup
	if rg == "" {
		t.Error("This test needs the env variable INTEGRATION_TEST_RESOURCE_GROUP_NAME to be set")
	}

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), functional.GetBicepRecipeRegistry(), functional.GetBicepRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "dsrp-resources-env-recipe-parameters-env",
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
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "mdb-param-ctnr").ValidateLabels(false),
					},
				},
			},
		},
	})

	test.Test(t)
}

// Test_MongoDB_Recipe_ContextParameter validates creation of a mongoDB from
// a default recipe using the context parameter generated and set by DatastoresRP,
// and container using the mongoDatabases portable resource to connect to the underlying mongoDB resource.
func Test_MongoDB_Recipe_ContextParameter(t *testing.T) {
	t.Skip("Skipping test as creating/deleting cosmosdb resource is unreliable - https://github.com/radius-project/radius/issues/5929")

	template := "testdata/datastoresrp-resources-mongodb-recipe-context.bicep"
	name := "dsrp-resources-mongodb-recipe-context"
	appNamespace := "dsrp-resources-mongodb-recipe-context-app"
	rg := os.Getenv("INTEGRATION_TEST_RESOURCE_GROUP_NAME")
	// Error the test if INTEGRATION_TEST_RESOURCE_GROUP_NAME is not set
	// for running locally set the INTEGRATION_TEST_RESOURCE_GROUP_NAME with the test resourceGroup
	if rg == "" {
		t.Error("This test needs the env variable INTEGRATION_TEST_RESOURCE_GROUP_NAME to be set")
	}

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), functional.GetBicepRecipeRegistry(), functional.GetBicepRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "dsrp-resources-env-recipes-context-env",
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
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "mdb-ctx-ctnr").ValidateLabels(false),
					},
				},
			},
		},
	})

	test.Test(t)
}
