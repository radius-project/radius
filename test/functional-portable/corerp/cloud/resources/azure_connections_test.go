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
	"fmt"
	"os"
	"testing"

	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
)

func Test_AzureConnections(t *testing.T) {
	name := "corerp-azure-connection-database-service"
	containerResourceName := "db-service"
	template := "testdata/corerp-azure-connection-database-service.bicep"
	appNamespace := name

	if os.Getenv("AZURE_COSMOS_MONGODB_ACCOUNT_ID") == "" {
		t.Error("AZURE_COSMOS_MONGODB_ACCOUNT_ID environment variable must be set to run this test.")
	}
	cosmosmongodbresourceid := "cosmosmongodbresourceid=" + os.Getenv("AZURE_COSMOS_MONGODB_ACCOUNT_ID")

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: containerResourceName,
						Type: validation.ComputeContainersResource,
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

	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, appNamespace)
	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployExecutor(template, testutil.GetMagpieImage(), cosmosmongodbresourceid, fmt.Sprintf("environment=%s", previewEnvID))

	test.RequiredFeatures = []rp.RequiredFeature{rp.FeatureAzure}
	test.Test(t)
}
