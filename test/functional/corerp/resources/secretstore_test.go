// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"context"
	"testing"

	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_SecretStore_CreateSecret(t *testing.T) {
	template := "testdata/corerp-resources-secretstore-new.bicep"
	appName := "corerp-resources-secretstore"
	appNamespace := "corerp-resources-secretstore-app"

	test := corerp.NewCoreRPTest(t, appNamespace, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, "@testdata/test-tls-cert.parameters.json"),
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
					{
						Name: "appsecret",
						Type: validation.SecretStoresResource,
						App:  appName,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sSecretForResource(appName, "appcert"),
						validation.NewK8sSecretForResource(appName, "appsecret"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test corerp.CoreRPTest) {
				secret, err := test.Options.K8sClient.CoreV1().Secrets(appNamespace).Get(ctx, "appcert", metav1.GetOptions{})
				require.NoError(t, err)

				for _, key := range []string{"tls.key", "tls.crt"} {
					_, ok := secret.Data[key]
					require.True(t, ok)
				}

				secret, err = test.Options.K8sClient.CoreV1().Secrets(appNamespace).Get(ctx, "appsecret", metav1.GetOptions{})
				require.NoError(t, err)

				for _, key := range []string{"servicePrincialPassword", "appId", "tenantId"} {
					_, ok := secret.Data[key]
					require.True(t, ok)
				}
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
