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
	name := "corerp-resources-sql-app"

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
						Name: "webapp",
						Type: validation.ContainersResource,
					},
					{
						Name: "sql-db",
						Type: validation.SQLDatabasesResource,
					},
					{
						Name: "sql-route",
						Type: validation.HttpRoutesResource,
					},
					{
						Name: "sql-container",
						Type: validation.ContainersResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "corerp-resources-sql-webapp"),
						validation.NewK8sPodForResource(name, "corerp-resources-sql-container"),
						validation.NewK8sServiceForResource(name, "corerp-resources-sql-route"),
					},
				},
			},
		},
	})

	test.Test(t)
}
