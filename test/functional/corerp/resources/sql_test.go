// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"testing"

	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

// FIXME: Test passes but containers are unhealthy.
func Test_SQL(t *testing.T) {
	t.Skip()
	template := "testdata/corerp-resources-sql.bicep"
	name := "corerp-resources-sql"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "sql-app-ctnr",
						Type: validation.ContainersResource,
					},
					{
						Name: "sql-db",
						Type: validation.SQLDatabasesResource,
					},
					{
						Name: "sql-rte",
						Type: validation.HttpRoutesResource,
					},
					{
						Name: "sql-ctnr",
						Type: validation.ContainersResource,
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
	})

	test.Test(t)
}
