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

func Test_SecretStore_CreateSecret(t *testing.T) {
	template := "testdata/corerp-resources-secretstore-new.bicep"
	appName := "corerp-resources-secretstore"
	appNamespace := "corerp-resources-secretstore-app"

	test := corerp.NewCoreRPTest(t, appNamespace, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, "@testdata/parameters/test-tls-cert.parameters.json"),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: appName,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "appcert",
						Type: validation.SecretStoresResource,
						App:  appName,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sSecretForResource(appName, "appcert"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_SecretStore_ReferenceSecret(t *testing.T) {
	template := "testdata/corerp-resources-secretstore-ref.bicep"
	appName := "corerp-resources-secretstore-ref"
	appNamespace := "corerp-resources-secretstore-ref"

	secret := corerp.K8sSecretResource("default", "secret-app-existing-secret", "kubernetes.io/tls", "tls.crt", "fakecertval", "tls.key", "fakekeyval")

	test := corerp.NewCoreRPTest(t, appNamespace, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: appName,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "existing-appcert",
						Type: validation.SecretStoresResource,
						App:  appName,
					},
				},
			},
			SkipObjectValidation: true,
		},
	}, secret)

	test.Test(t)
}
