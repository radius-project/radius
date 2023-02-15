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

	if os.Getenv("MSSQL_RESOURCE_ID") == "" {
		t.Error("MSSQL_RESOURCE_ID environment variable must be set to run this test.")
	}
	if os.Getenv("MSSQL_USERNAME") != "" && os.Getenv("MSSQL_PASSWORD") != "" {
		adminUsername = "adminUsername=" + os.Getenv("MSSQL_USERNAME")
		adminPassword = "adminPassword=" + os.Getenv("MSSQL_PASSWORD")
	} else {
		t.Error("MSSQL_USERNAME and MSSQL_PASSWORD environment variable must be set to run this test.")
	}
	mssqlresourceid := "mssqlresourceid=" + os.Getenv("MSSQL_RESOURCE_ID")
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
