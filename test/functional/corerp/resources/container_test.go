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
	t.Skip()

	template := "testdata/corerp-resources-container.bicep"
	name := "corerp-resources-container"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
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
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					name: {
						validation.NewK8sPodForResource(name, "corerp-resources-container-container"),
						validation.NewK8sHTTPProxyForResource(name, "corerp-resources-container-httproute"),
					},
				},
			},
			SkipObjectValidation: true,
		},
	})

	test.Test(t)
}
