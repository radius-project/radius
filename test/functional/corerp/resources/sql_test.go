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

func Test_SQL(t *testing.T) {
	t.Skip()

	template := "testdata/corerp-resources-rabbitmq.bicep"
	name := "corerp-resources-rabbitmq"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-sql-app",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "corerp-resources-sql-webapp",
						Type: validation.ContainersResource,
					},
					{
						Name: "corerp-resources-sql-db",
						Type: validation.SQLDatabasesResource,
					},
					{
						Name: "corerp-resources-sql-route",
						Type: validation.HttpRoutesResource,
					},
					{
						Name: "corerp-resources-sql-container",
						Type: validation.ContainersResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					name: {
						validation.NewK8sPodForResource(name, "corerp-resources-sql-webapp"),
						validation.NewK8sPodForResource(name, "corerp-resources-sql-container"),
						validation.NewK8sHTTPProxyForResource(name, "corerp-resources-sql-route"),
					},
				},
			},
			SkipObjectValidation: true,
		},
	})

	test.Test(t)
}
