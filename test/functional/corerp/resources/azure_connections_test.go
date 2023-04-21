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

func Test_AzureConnections(t *testing.T) {
	t.Skip()
	name := "corerp-azure-connection-database-service"
	containerResourceName := "db-service"
	template := "testdata/corerp-azure-connection-database-service.bicep"

	if os.Getenv("AZURE_COSMOS_MONGODB_ACCOUNT_ID") == "" {
		t.Error("AZURE_COSMOS_MONGODB_ACCOUNT_ID environment variable must be set to run this test.")
	}
	cosmosmongodbresourceid := "cosmosmongodbresourceid=" + os.Getenv("AZURE_COSMOS_MONGODB_ACCOUNT_ID")
	appNamespace := "default-corerp-azure-connection-database-service"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), cosmosmongodbresourceid),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: containerResourceName,
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, containerResourceName),
					},
				},
			},
		},
	})

	test.Test(t)
}
