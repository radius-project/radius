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

// FIXME: App is not available.
// Failed to load logs: container "app" in pod "msql-app-85877f5fdb-q9rxj" is waiting to start: CreateContainerConfigError
func Test_MicrosoftSQL(t *testing.T) {
	t.Skip()
	template := "testdata/corerp-resources-microsoft-sql.bicep"
	name := "corerp-resources-microsoft-sql"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-microsoft-sql",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "app",
						Type: validation.ContainersResource,
					},
					{
						Name: "db",
						Type: validation.SQLDatabasesResource,
					},
					{
						Name: "route",
						Type: validation.HttpRoutesResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "app"),
						validation.NewK8sPodForResource(name, "db"),
						validation.NewK8sServiceForResource(name, "route"),
					},
				},
			},
		},
	})

	test.Test(t)
}
