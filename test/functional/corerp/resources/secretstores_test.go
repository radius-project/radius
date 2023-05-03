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

func Test_SecretStores_CreateSecret(t *testing.T) {
	template := "testdata/corerp-resources-secretstores-value.bicep"
	name := "corerp-resources-secret-app"
	appNamespace := "corerp-resources-secret-app"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, "@testdata/test-tls-cert.parameters.json", functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "appcert",
						Type: validation.SecretStoresResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sSecretForResource(name, "appcert"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_SecretStores_ReferenceSecret(t *testing.T) {
	template := "testdata/corerp-resources-secretstores-valuefrom.bicep"
	name := "corerp-resources-secret-app"
	appNamespace := "corerp-resources-secret-app"

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
						Name: "existing-appcert",
						Type: validation.SecretStoresResource,
						App:  name,
					},
				},
			},
			SkipObjectValidation: true,
		},
	}, corerp.K8sSecretResource(appNamespace, "secret-app-existing-secret", "tls.crt", "fakecertval", "tls.key", "fakekeyval"))

	test.Test(t)
}
