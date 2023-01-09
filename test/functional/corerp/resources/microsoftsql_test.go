// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"os"
	"testing"

	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_MicrosoftSQL(t *testing.T) {
	template := "testdata/corerp-resources-microsoft-sql.bicep"
	name := "corerp-resources-microsoft-sql"

	var adminUsername, adminPassword string
	mssqlresourceid := "mssqlresourceid=" + os.Getenv("MSSQL_RESOURCE_ID")
	if mssqlresourceid == "" {
		t.Error("failed to get mssqlresource id from the environment")
	}
	if os.Getenv("MSSQL_USERNAME") != "" && os.Getenv("MSSQL_PASSWORD") != "" {
		adminUsername = "adminUsername=" + os.Getenv("MSSQL_USERNAME")
		adminPassword = "adminPassword=" + os.Getenv("MSSQL_PASSWORD")
	} else {
		t.Error("failed to get msql username or password  from the environment")
	}

	appNamespace := "default-corerp-resources-microsoft-sql"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), mssqlresourceid, adminUsername, adminPassword),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
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
