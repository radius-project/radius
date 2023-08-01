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
	"os"
	"testing"

	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/shared"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_AzureConnections(t *testing.T) {
	name := "corerp-azure-connection-database-service"
	containerResourceName := "db-service"
	template := "testdata/corerp-azure-connection-database-service.bicep"

	if os.Getenv("AZURE_COSMOS_MONGODB_ACCOUNT_ID") == "" {
		t.Error("AZURE_COSMOS_MONGODB_ACCOUNT_ID environment variable must be set to run this test.")
	}
	cosmosmongodbresourceid := "cosmosmongodbresourceid=" + os.Getenv("AZURE_COSMOS_MONGODB_ACCOUNT_ID")
	appNamespace := "default-corerp-azure-connection-database-service"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), cosmosmongodbresourceid),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
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
