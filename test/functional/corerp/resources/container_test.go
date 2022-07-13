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
	template := "testdata/corerp-resources-container.bicep"
	appName := "corerp-resources-container"

	test := corerp.NewCoreRPTest(t, appName, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-container-app",
						Type: validation.ApplicationsResource,
					},
					{
						Name:    "container",
						Type:    validation.ContainersResource,
						AppName: "corerp-resources-container-app",
					},
					// {
					// 	Name: "corerp-resources-container-httproute",
					// 	Type: validation.HttpRoutesResource,
					// },
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(appName, "container"),
						// validation.NewK8sHTTPProxyForResource(appName, "corerp-resources-container-httproute"),
					},
				},
			},
		},
	})

	test.Test(t)
}
