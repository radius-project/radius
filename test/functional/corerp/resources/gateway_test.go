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

func Test_Gateway(t *testing.T) {
	template := "testdata/corerp-resources-gateway.bicep"
	name := "corerp-resources-gateway-app"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-gateway-app",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "corerp-resources-gateway-app-gateway",
						Type: validation.GatewaysResource,
					},
					{
						Name: "corerp-resources-gateway-app-frontend-route",
						Type: validation.HttpRoutesResource,
					},
					{
						Name: "corerp-resources-gateway-app-frontend-container",
						Type: validation.ContainersResource,
					},
					{
						Name: "corerp-resources-gateway-app-backend-route",
						Type: validation.HttpRoutesResource,
					},
					{
						Name: "corerp-resources-gateway-app-backend-container",
						Type: validation.ContainersResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "frontend-container"),
						validation.NewK8sPodForResource(name, "backend-container"),
						validation.NewK8sHTTPProxyForResource(name, "gateway"),
						validation.NewK8sHTTPProxyForResource(name, "frontend-route"),
						validation.NewK8sServiceForResource(name, "frontend-route"),
						validation.NewK8sHTTPProxyForResource(name, "backend-route"),
						validation.NewK8sServiceForResource(name, "backend-route"),
					},
				},
			},
		},
	})

	test.Test(t)
}
