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

func Test_ApplicationAndEnvironment(t *testing.T) {
	t.Skip()

	template := "testdata/corerp-resources-app-env.bicep"
	name := "corerp-resources-app-env"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-app-env-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: "corerp-resources-app-env-app",
						Type: validation.ApplicationsResource,
					},
				},
			},
			SkipObjectValidation: true,
		},
	})

	test.Test(t)
}
