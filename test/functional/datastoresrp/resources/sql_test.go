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
	"runtime"
	"testing"

	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/shared"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_SQLDatabase_Manual(t *testing.T) {
	// https://github.com/microsoft/mssql-docker/issues/668
	if runtime.GOARCH == "arm64" {
		t.Skip("skipping Test_SQL, unsupported architecture")
	}
	template := "testdata/datastoresrp-resources-sqldb-manual.bicep"
	name := "dsrp-resources-sql"
	appNamespace := "default-dsrp-resources-sql"

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
						Name: "sql-app-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "sql-db",
						Type: validation.SQLDatabasesResource,
						App:  name,
					},
					{
						Name: "sql-rte",
						Type: validation.HttpRoutesResource,
						App:  name,
					},
					{
						Name: "sql-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "sql-app-ctnr"),
						validation.NewK8sPodForResource(name, "sql-ctnr"),
						validation.NewK8sServiceForResource(name, "sql-rte"),
					},
				},
			},
		},
	})

	test.Test(t)
}

// Test_SQLDatabase_Recipe validates:
// the creation of a sql database from recipe
// container using the sqlDatabases portable resource to connect to the sql database resource
func Test_SQLDatabase_Recipe(t *testing.T) {
	template := "testdata/datastoresrp-resources-sqldb-recipe.bicep"
	name := "dsrp-resources-sqldb-recipe"
	appNamespace := "dsrp-resources-sqldb-recipe-app"
	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), functional.GetBicepRecipeRegistry(), functional.GetBicepRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "dsrp-resources-env-sql-recipe-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: "dsrp-resources-sqldb-recipe",
						Type: validation.ApplicationsResource,
						App:  name,
					},
					{
						Name: "sql-recipe-app-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "sql-recipe-app-ctnr").ValidateLabels(false),
						validation.NewK8sPodForResource(name, "sql-recipe-resource").ValidateLabels(false),
					},
				},
			},
		},
	})

	test.Test(t)
}
