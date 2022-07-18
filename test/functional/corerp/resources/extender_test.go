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

func Test_Extender(t *testing.T) {
	template := "testdata/corerp-resources-extender.bicep"
	name := "corerp-resources-extender"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-extender",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "myapp",
						Type: validation.ContainersResource,
					},
					{
						Name: "twilio",
						Type: validation.HttpRoutesResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "myapp"),
					},
				},
			},
		},
	})

	test.Test(t)
}
