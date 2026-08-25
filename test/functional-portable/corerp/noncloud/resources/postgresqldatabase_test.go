/*
Copyright 2026 The Radius Authors.

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
	"github.com/radius-project/radius/test/validation"
)

// Test_PostgreSQLDatabase deploys a Radius.Data/postgreSqlDatabases resource using the
// default preview-environment recipe pack and validates the Kubernetes resources
// provisioned by the recipe.
func Test_PostgreSQLDatabase(t *testing.T) {
	template := "testdata/corerp-resources-postgresqldatabase.bicep"
	name := "corerp-resources-postgresqldb"
	appNamespace := "corerp-resources-postgresqldb"
	databaseName := "postgresqldb-db"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: databaseName,
						Type: validation.DataPostgreSQLDatabasesResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, databaseName),
						validation.NewK8sDeploymentForResource(name, databaseName),
						validation.NewK8sServiceForResource(name, databaseName),
					},
				},
			},
		},
	})

	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, appNamespace)
	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployExecutor(
		template,
		fmt.Sprintf("environment=%s", previewEnvID),
		"password=******",
	)

	test.Test(t)
}
