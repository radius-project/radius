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
	template := "testdata/corerp-resources-app-env.bicep"
	name := "corerp-resources-app-env"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewTempCoreRPExecutor(template),
			Resources: []validation.Resource{
				{
					Name: "corerp-resources-application-env",
					Type: validation.EnvironmentsResource,
				},
				{
					Name: "corerp-resources-application-app",
					Type: validation.ApplicationsResource,
				},
			},
		},
	})

	test.Test(t)
}
