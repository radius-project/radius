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
	"fmt"
	"testing"

	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
)

// Test_MySQLDatabase deploys a Radius.Data/mySqlDatabases resource using the default
// preview-environment recipe pack and validates that the recipe-provisioned MySQL
// Deployment/Service has a running Pod, and that a Radius.Compute/containers resource is
// deployed with a connection to the database.
func Test_MySQLDatabase(t *testing.T) {
	template := "testdata/corerp-resources-mysqldatabase.bicep"
	name := "corerp-resources-mysqldb"
	appNamespace := "corerp-resources-mysqldb"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: "mysqldb-db",
						Type: validation.DataMySQLDatabasesResource,
						App:  name,
					},
					{
						Name: "mysqldb-ctnr",
						Type: validation.ComputeContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						// The application container.
						validation.NewK8sPodForResource(name, "mysqldb-ctnr"),
						validation.NewK8sServiceForResource(name, "mysqldb-ctnr"),
						// The MySQL Deployment and Service provisioned by the recipe.
						validation.NewK8sPodForResource(name, "mysqldb-db"),
						validation.NewK8sServiceForResource(name, "mysqldb-db"),
					},
				},
			},
		},
	})

	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, appNamespace)
	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployExecutor(template, testutil.GetMagpieImage(), fmt.Sprintf("environment=%s", previewEnvID), fmt.Sprintf("password=%s", "not-prod-password"))

	test.Test(t)
}
