// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"testing"

	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_AzureConnections(t *testing.T) {
	t.Skip("Will re-enable after all components are completed for Private Preview. Ref: https://github.com/project-radius/radius/issues/2736")

	name := "corerp-connection-database-service"
	containerResourceName := "db-service"
	template := "testdata/corerp-connection-database-service.bicep"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: containerResourceName,
						Type: validation.ContainersResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					name: {
						validation.NewK8sPodForResource(name, containerResourceName),
					},
				},
			},
		},
	})

	test.Test(t)
}
