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

func Test_ApplicationAndEnvironment2(t *testing.T) {
	template := "testdata/corerp-resources-kubernetesmetadata.bicep"
	name := "corerp-kubemetadata"

	requiredSecrets := map[string]map[string]string{}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-kubemetadata-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: "corerp-kubemetadata-app",
						Type: validation.ApplicationsResource,
					},
				},
			},
			// Application and Environment should not render any K8s Objects directly
			K8sObjects: &validation.K8sObjectSet{},
		},
	}, requiredSecrets)

	test.Test(t)
}
