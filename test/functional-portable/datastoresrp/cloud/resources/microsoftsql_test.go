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
	"testing"

	"github.com/google/uuid"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
)

func Test_MicrosoftSQL_Manual(t *testing.T) {
	template := "testdata/datastoresrp-resources-microsoft-sql.bicep"
	name := "dsrp-resources-microsoft-sql"

	sqlDatabase := "database=database-" + uuid.New().String()
	sqlServer := "server=server-" + uuid.New().String()
	adminUsername := "adminUsername=adminUsername-" + uuid.New().String()
	adminPassword := "adminPassword=" + uuid.New().String()
	appNamespace := "default-dsrp-resources-microsoft-sql"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetMagpieImage(), adminUsername, adminPassword, sqlDatabase, sqlServer),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
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

	test.RequiredFeatures = []rp.RequiredFeature{rp.FeatureAzure}
	test.Test(t)
}
