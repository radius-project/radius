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
	template := "testdata/corerp-resources-environment.bicep"
	name := "corerp-resources-environment"

	test := corerp.NewApplicationTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewTempCoreRPExecutor(template),
			Resources: []validation.Resource{
				{
					Name: "corerp-resources-environment",
					Type: validation.EnvironmentsResource,
				},
			},
		},
	})

	test.Test(t)
}
