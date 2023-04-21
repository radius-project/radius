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

func Test_Environment(t *testing.T) {
	t.Skip()
	template := "testdata/corerp-resources-environment.bicep"
	name := "corerp-resources-environment"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-environment-env",
						Type: validation.EnvironmentsResource,
					},
				},
			},
			// Environment should not render any K8s Objects directly
			K8sObjects: &validation.K8sObjectSet{},
		},
	})

	test.Test(t)
}
