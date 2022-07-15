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

func Test_MicrosoftSQL(t *testing.T) {
	t.Skip()

	template := "corerp-resources-microsoft-sql.bicep"
	name := "corerp-resources-microsoft-sql"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-microsoft-app",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "corerp-resources-microsoft-sqlapp",
						Type: validation.ContainersResource,
					},
					{
						Name: "corerp-resources-microsoft-db",
						Type: validation.SQLDatabasesResource,
					},
					{
						Name: "corerp-resources-microsoft-route",
						Type: validation.HttpRoutesResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					name: {
						validation.NewK8sPodForResource(name, "corerp-resources-microsoft-sqlapp"),
						validation.NewK8sPodForResource(name, "corerp-resources-microsoft-db"),
						validation.NewK8sHTTPProxyForResource(name, "corerp-resources-sql-route"),
					},
				},
			},
			SkipObjectValidation: true,
		},
	})

	test.Test(t)
}
