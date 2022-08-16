// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"runtime"
	"testing"

	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_SQL(t *testing.T) {
	// https://github.com/microsoft/mssql-docker/issues/668
	if runtime.GOARCH == "arm64" {
		t.Skip("skipping Test_SQL, unsupported architecture")
	}
	template := "testdata/corerp-resources-sql.bicep"
	name := "corerp-resources-sql"

	requiredSecrets := map[string]map[string]string{}

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
					"default": {
						validation.NewK8sPodForResource(name, "sql-app-ctnr"),
						validation.NewK8sPodForResource(name, "sql-ctnr"),
						validation.NewK8sServiceForResource(name, "sql-rte"),
					},
				},
			},
		},
	}, requiredSecrets)

	test.Test(t)
}
