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
	appName := "corerp-resources-container-app"

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
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(appName, "container"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_ContainerHttpRoute(t *testing.T) {
	template := "testdata/corerp-resources-container-httproute.bicep"
	appName := "corerp-resources-container-httproute-app"

	test := corerp.NewCoreRPTest(t, appName, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-container-httproute-app",
						Type: validation.ApplicationsResource,
					},
					{
						Name:    "container",
						Type:    validation.ContainersResource,
						AppName: "corerp-resources-container-httproute-app",
					},
					{
						Name:    "httproute",
						Type:    validation.HttpRoutesResource,
						AppName: "corerp-resources-container-httproute-app",
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(appName, "container"),
						validation.NewK8sServiceForResource(appName, "httproute"),
					},
				},
			},
		},
	})

	test.Test(t)
}
