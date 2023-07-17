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

	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/shared"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

// Opt-out case for manual resource provisioning
func Test_MongoDB_ManualProvisioning(t *testing.T) {
	template := "testdata/corerp-resources-mongodb-manual-provisioning.bicep"
	name := "corerp-resources-mongodb-manual-provisioning"
	appNamespace := "default-ccorerp-resources-mongodb-manual-provisioning"

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
						validation.NewK8sPodForResource(name, "mdb-us-app-ctnr").ValidateLabels(false),
						validation.NewK8sPodForResource(name, "mdb-us-ctnr").ValidateLabels(false),
						validation.NewK8sServiceForResource(name, "mdb-us-rte").ValidateLabels(false),
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
	template := "testdata/corerp-resources-mongodb-recipe.bicep"
	name := "corerp-resources-mongodb-recipe"
	appNamespace := "corerp-resources-mongodb-recipe-app"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), functional.GetRecipeRegistry(), functional.GetRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "corerp-resources-mongodb-recipe-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: "corerp-resources-mongodb-recipe",
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
// and developer while creating the mongoDatabase link.
// If the same parameters are set by the developer and the operator then the developer parameters are applied in to resolve conflicts.
// Container uses the mongoDB link to connect to the mongoDB resource
func Test_MongoDB_RecipeParameters(t *testing.T) {
	template := "testdata/corerp-resources-mongodb-recipe-parameters.bicep"
	name := "corerp-resources-mongodb-recipe-parameters"
	appNamespace := "corerp-resources-mongodb-recipe-param-app"
	rg := os.Getenv("INTEGRATION_TEST_RESOURCE_GROUP_NAME")
	// Error the test if INTEGRATION_TEST_RESOURCE_GROUP_NAME is not set
	// for running locally set the INTEGRATION_TEST_RESOURCE_GROUP_NAME with the test resourceGroup
	if rg == "" {
		t.Error("This test needs the env variable INTEGRATION_TEST_RESOURCE_GROUP_NAME to be set")
	}

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), functional.GetRecipeRegistry(), functional.GetRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
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
								LocalID:  "RecipeResource0",
							},
							{
								Provider: resourcemodel.ProviderAzure,
								LocalID:  "RecipeResource1",
							},
						},
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
// a default recipe using the context parameter generated and set by linkRP,
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

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), functional.GetRecipeRegistry(), functional.GetRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
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
								LocalID:  "RecipeResource0",
							},
							{
								Provider: resourcemodel.ProviderAzure,
								LocalID:  "RecipeResource1",
							},
						},
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
