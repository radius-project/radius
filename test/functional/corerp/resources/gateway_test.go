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
	t.Skip()

	template := "testdata/corerp-resources-gateway.bicep"
	name := "corerp-resources-gateway"

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
						Name: "corerp-resources-gateway-gateway",
						Type: validation.GatewaysResource,
					},
					{
						Name: "corerp-resources-gateway-frontend-route",
						Type: validation.HttpRoutesResource,
					},
					{
						Name: "corerp-resources-gateway-frontend-container",
						Type: validation.ContainersResource,
					},
					{
						Name: "corerp-resources-gateway-backend-route",
						Type: validation.HttpRoutesResource,
					},
					{
						Name: "corerp-resources-gateway-backend-container",
						Type: validation.ContainersResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					name: {
						validation.NewK8sPodForResource(name, "corerp-resources-gateway-frontend"),
						validation.NewK8sPodForResource(name, "corerp-resources-gateway-backend"),
						validation.NewK8sHTTPProxyForResource(name, "corerp-resources-gateway-gateway"),
						validation.NewK8sHTTPProxyForResource(name, "corerp-resources-gateway-frontendroute"),
						validation.NewK8sServiceForResource(name, "corerp-resources-gateway-frontendroute"),
						validation.NewK8sHTTPProxyForResource(name, "corerp-resources-gateway-backendroute"),
						validation.NewK8sServiceForResource(name, "corerp-resources-gateway-backendroute"),
					},
				},
			},
			SkipObjectValidation: true,
		},
	})

	test.Test(t)
}
