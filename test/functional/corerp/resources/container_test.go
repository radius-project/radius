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

func Test_Container(t *testing.T) {
	t.Skip("Will re-enable after: https://github.com/project-radius/deployment-engine/issues/146")

	template := "testdata/corerp-resources-container.bicep"
	name := "corerp-resources-container"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			Resources: []validation.Resource{
				{
					Name: "corerp-resources-container-app",
					Type: validation.ApplicationsResource,
				},
				{
					Name: "corerp-resources-container-container",
					Type: validation.ContainersResource,
				},
				{
					Name: "corerp-resources-container-httproute",
					Type: validation.HttpRoutesResource,
				},
			},
		},
	})

	test.Test(t)
}
