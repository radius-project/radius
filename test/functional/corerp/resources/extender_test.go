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
	t.Skip()
	template := "testdata/corerp-resources-extender.bicep"
	name := "corerp-resources-extender"
	appNamespace := "default-corerp-resources-extender"

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
						Name: "extr-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "extr-twilio",
						Type: validation.ExtendersResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "extr-ctnr"),
					},
				},
			},
		},
	})

	test.Test(t)
}
