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
	t.Skip("Will re-enable after: https://github.com/project-radius/deployment-engine/issues/146")

	template := "testdata/corerp-resources-app-env.bicep"
	name := "corerp-resources-app-env"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewTempCoreRPExecutor(template),
			Resources: []validation.Resource{
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
	})

	test.Test(t)
}
