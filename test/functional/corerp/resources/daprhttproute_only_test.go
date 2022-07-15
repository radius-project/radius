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

func Test_DaprHttpRouteConnector(t *testing.T) {
	template := "testdata/connectorrp-resources-dapr-http-route.bicep"
	name := "connectorrp-resources-dapr-http-route"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "connectorrp-resources-dapr-http-route",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "httproute",
						Type: validation.DaprInvokeHttpRouteResource,
					},
				},
			},
		},
	})

	test.Test(t)
}
