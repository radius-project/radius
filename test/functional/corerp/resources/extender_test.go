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

func Test_Extender(t *testing.T) {
	t.Skip("https://github.com/project-radius/radius/issues/3182")
	template := "testdata/corerp-resources-extender.bicep"
	name := "corerp-resources-extender"

	requiredSecrets := map[string]map[string]string{}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-extender",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "extr-ctnr",
						Type: validation.ContainersResource,
					},
					{
						Name: "extr-twilio",
						Type: validation.HttpRoutesResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "extr-ctnr"),
					},
				},
			},
		},
	}, requiredSecrets)

	test.Test(t)
}
