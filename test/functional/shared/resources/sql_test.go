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

	"github.com/radius-project/radius/test/functional"
	"github.com/radius-project/radius/test/functional/shared"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"
)

func Test_SQL(t *testing.T) {
	// https://github.com/microsoft/mssql-docker/issues/668
	if runtime.GOARCH == "arm64" {
		t.Skip("skipping Test_SQL, unsupported architecture")
	}
	template := "testdata/corerp-resources-sql.bicep"
	name := "corerp-resources-sql"
	appNamespace := "default-corerp-resources-sql"

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
						Name: "sql-app-ctnr-old",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "sql-db-old",
						Type: validation.O_SQLDatabasesResource,
						App:  name,
					},
					{
						Name: "sql-rte-old",
						Type: validation.HttpRoutesResource,
						App:  name,
					},
					{
						Name: "sql-ctnr-old",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "sql-app-ctnr-old"),
						validation.NewK8sPodForResource(name, "sql-ctnr-old"),
						validation.NewK8sServiceForResource(name, "sql-rte-old"),
					},
				},
			},
		},
	})

	test.Test(t)
}
