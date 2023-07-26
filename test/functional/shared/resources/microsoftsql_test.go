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

	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/shared"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_MicrosoftSQL(t *testing.T) {
	template := "testdata/corerp-resources-microsoft-sql.bicep"
	name := "corerp-resources-microsoft-sql"

	var adminUsername, adminPassword string

	if os.Getenv("AZURE_MSSQL_RESOURCE_ID") == "" {
		t.Error("AZURE_MSSQL_RESOURCE_ID environment variable must be set to run this test.")
	}
	if os.Getenv("AZURE_MSSQL_DATABASE") == "" || os.Getenv("AZURE_MSSQL_SERVER") == "" {
		t.Error("AZURE_MSSQL_DATABASE and AZURE_MSSQL_SERVER environment variable must be set to run this test.")
	}
	if os.Getenv("AZURE_MSSQL_USERNAME") != "" && os.Getenv("AZURE_MSSQL_PASSWORD") != "" {
		adminUsername = "adminUsername=" + os.Getenv("AZURE_MSSQL_USERNAME")
		adminPassword = "adminPassword=" + os.Getenv("AZURE_MSSQL_PASSWORD")
	} else {
		t.Error("AZURE_MSSQL_USERNAME and AZURE_MSSQL_PASSWORD environment variable must be set to run this test.")
	}
	mssqlresourceid := "mssqlresourceid=" + os.Getenv("AZURE_MSSQL_RESOURCE_ID")
	sqlDatabse := "database=" + os.Getenv("AZURE_MSSQL_DATABASE")
	sqlServer := "server=" + os.Getenv("AZURE_MSSQL_SERVER")
	appNamespace := "default-corerp-resources-microsoft-sql"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), mssqlresourceid, adminUsername, adminPassword, sqlDatabse, sqlServer),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "mssql-app-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "mssql-app-ctnr"),
					},
				},
			},
		},
	})

	test.Test(t)
}

// Test_SQLDatabase_Recipe validates:
// the creation of a sql database from recipe
// container using the sql database link to connect to the sql database resource
func Test_SQLDatabase_Recipe(t *testing.T) {
	template := "testdata/corerp-resources-sqldb-recipe.bicep"
	name := "corerp-resources-sqldb-recipe"
	appNamespace := "corerp-resources-sqldb-recipe-app"
	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), functional.GetBicepRecipeRegistry(), functional.GetBicepRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "corerp-resources-environment-sql-recipe-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: "corerp-resources-sqldb-recipe",
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
