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

// FIXME: Frontend container logs this:
// 2022/07/16 22:38:39 no provider could be found for binding of type -  <nil>
// 2022/07/16 22:38:47 no provider could be found for binding of type -  <nil>
func Test_Gateway(t *testing.T) {
	t.Skip("https://github.com/project-radius/radius/issues/3182")
	template := "testdata/corerp-resources-gateway.bicep"
	name := "corerp-resources-gateway"

	requiredSecrets := map[string]map[string]string{}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "gtwy-gtwy",
						Type: validation.GatewaysResource,
					},
					{
						Name: "gtwy-front-rte",
						Type: validation.HttpRoutesResource,
					},
					{
						Name: "gtwy-front-ctnr",
						Type: validation.ContainersResource,
					},
					{
						Name: "gtwy-back-rte",
						Type: validation.HttpRoutesResource,
					},
					{
						Name: "gtwy-back-ctnr",
						Type: validation.ContainersResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "gtwy-front-ctnr"),
						validation.NewK8sPodForResource(name, "gtwy-back-ctnr"),
						validation.NewK8sHTTPProxyForResource(name, "gtwy-gtwy"),
						validation.NewK8sHTTPProxyForResource(name, "gtwy-front-rte"),
						validation.NewK8sServiceForResource(name, "gtwy-front-rte"),
						validation.NewK8sHTTPProxyForResource(name, "gtwy-back-rte"),
						validation.NewK8sServiceForResource(name, "gtwy-back-rte"),
					},
				},
			},
		},
	}, requiredSecrets)

	test.Test(t)
}
