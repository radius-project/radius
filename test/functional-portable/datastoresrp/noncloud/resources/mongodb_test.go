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

	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
)

// Opt-out case for manual resource provisioning
func Test_MongoDB_Manual(t *testing.T) {
	template := "testdata/datastoresrp-rs-mongodb-manual.bicep"
	name := "dsrp-resources-mongodb-manual"
	appNamespace := "default-cdsrp-resources-mongodb-manual"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetMagpieImage()),
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

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetMagpieImage(), testutil.GetBicepRecipeRegistry(), testutil.GetBicepRecipeVersion()),
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
