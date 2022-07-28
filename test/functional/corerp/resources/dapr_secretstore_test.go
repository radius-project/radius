// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"context"
	"os/exec"
	"testing"

	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_DaprSecretStoreGeneric(t *testing.T) {
	//t.Skip("https://github.com/project-radius/radius/issues/3182")

	template := "testdata/corerp-resources-dapr-secretstore-generic.bicep"
	name := "corerp-resources-dapr-secretstore-generic"

	requiredSecrets := map[string]map[string]string{
		"newsecret": {
			"newsecret": "newsecret",
		},
	}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-dapr-secretstore-generic",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "gnrc-scs-ctnr",
						Type: validation.ContainersResource,
					},
					{
						Name: "gnrc-scs",
						Type: validation.DaprSecretStoreResource,
					},
				},
			},
			// K8sObjects: &validation.K8sObjectSet{
			// 	Namespaces: map[string][]validation.K8sObject{
			// 		"default": {
			// 			validation.NewK8sPodForResource(name, "gnrc-scs-ctnr"),
			// 		},
			// 	},
			// },
			PostStepVerify: func(ctx context.Context, t *testing.T, test corerp.CoreRPTest) {
				exec.Command("kubectl", "describe", "components")
			},
		},
	}, requiredSecrets)

	test.Test(t)
}
