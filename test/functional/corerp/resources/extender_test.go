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
	t.Skip("Currently disabled and needs validation. Once corresponding Core RP feature lights up, re-enable this test and see if it succeeds.")

	template := "testdata/corerp-resources-extender.bicep"
	name := "corerp-resources-extender"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			Resources: []validation.Resource{
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
	})

	test.Test(t)
}
