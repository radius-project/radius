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

func Test_DaprInvokeHttpRoute(t *testing.T) {
	template := "testdata/corerp-resources-dapr-httproute.bicep"
	name := "dapr-invokehttproute"

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
						Name: "dapr-frontend",
						Type: validation.ContainersResource,
					},
					{
						Name: "dapr-backend",
						Type: validation.ContainersResource,
					},
					{
						Name: "dapr-backend-httproute",
						Type: validation.DaprInvokeHttpRoute,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "dapr-frontend"),
						validation.NewK8sPodForResource(name, "dapr-backend"),
					},
				},
			},
		},
	}, requiredSecrets)

	test.Test(t)
}
