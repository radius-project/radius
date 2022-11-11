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

func Test_DaprSecretStoreGeneric(t *testing.T) {
	template := "testdata/corerp-resources-dapr-secretstore-generic.bicep"
	name := "corerp-resources-dapr-secretstore-generic"

	requiredSecrets := map[string]map[string]string{
		"mysecret": {
			"mysecret": "mysecret",
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
						App:  name,
					},
					{
						Name: "gnrc-scs",
						Type: validation.DaprSecretStoresResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "gnrc-scs-ctnr"),
					},
				},
			},
			// TODO: Remove the below when https://github.com/project-radius/radius/issues/4627 is fixed.
			SkipResourceDeletion: true,
		},
	}, requiredSecrets)

	// TODO: Remove the below and "SkipResourceDeletion: true" when https://github.com/project-radius/radius/issues/4627 is fixed.
	test.SkipSecretDeletion = true

	test.Test(t)
}
