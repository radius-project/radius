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

func TestK8sModule(t *testing.T) {
	t.Skip("Currently disabled and needs validation. Once corresponding Core RP feature lights up, re-enable this test and see if it succeeds.")

	template := "testdata/corerp-module/corerp-main.bicep"
	application := "corerp-module"

	test := corerp.NewCoreRPTest(t, application, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			Resources: []validation.Resource{
				{
					Name: "corerp-resources-extender",
					Type: validation.ApplicationsResource,
				},
				{
					Name: "busybox",
					Type: validation.ContainersResource,	
				},
				{
					Name: "container",
					Type: validation.ContainersResource,
				},
			},
		},
	})
	test.Test(t)
}
