// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mechanics_test

import (
	"testing"

	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_NestedModules(t *testing.T) {
	template := "testdata/corerp-mechanics-nestedmodules.bicep"
	name := "corerp-mechanics-nestedmodules"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-mechanics-nestedmodules-outerapp-app",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "corerp-mechanics-nestedmodules-innerapp-app",
						Type: validation.ApplicationsResource,
					},
				},
			},
			SkipObjectValidation: true,
		},
	})

	test.Test(t)
}
