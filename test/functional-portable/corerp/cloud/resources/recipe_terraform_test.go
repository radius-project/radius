/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resource_test

// This file contains tests for Terraform recipes functionality - covering general behaviors that should
// be consistent across all resource types.
//
// New tests using cloud resources should be added if absolutely necessary.
//
// Tests in this file should be kept *roughly* in sync with recipe_bicep_test and any other drivers.

import (
	"context"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/v3"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/radius-project/radius/pkg/recipes/terraform/config/providers"
	"github.com/radius-project/radius/test/functional-portable/corerp"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
)

var (
	secretNamespace = "radius-system"
	secretPrefix    = "tfstate-default-"
)

// Test_TerraformRecipe_AzureResourceGroup checks default and custom provider configuration through deployment and destruction.
func Test_TerraformRecipe_AzureResourceGroup(t *testing.T) {
	for _, custom := range []bool{false, true} {
		label := "default"
		if custom {
			label = "custom"
		}
		t.Run(label, func(t *testing.T) {
			if custom {
				t.Skip("Terraform get rejects modules with provider configuration_aliases: https://github.com/radius-project/radius/issues/13299")
			}
			ctx := t.Context()
			name := "corerp-resources-terraform-azrg-" + label
			appName, envName := name+"-app", name+"-env"
			groupName := "tfrg-" + uuid.New().String()
			subscription := requiredCloudEnv(t, "AZURE_SUBSCRIPTION_ID")
			groups, err := armresources.NewResourceGroupsClient(subscription, azureTestCredential(ctx, t), nil)
			require.NoError(t, err)

			// Fallback cleanup runs after lifecycle assertions, so it cannot hide a failed destroy.
			t.Cleanup(func() {
				cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
				defer cancel()
				poller, err := groups.BeginDelete(cleanupCtx, groupName, nil)
				if isNotFoundResponse(err) {
					return
				}
				if err == nil {
					_, err = poller.PollUntilDone(cleanupCtx, nil)
				}
				if err != nil {
					t.Errorf("delete test Azure resource group %s: %v", groupName, err)
				}
			})

			parameters := []string{
				testutil.GetTerraformRecipeModuleServerURL(), "appName=" + appName, "envName=" + envName,
				"resourceName=" + name, "azureResourceGroupName=" + groupName,
			}
			template := "testdata/corerp-resources-terraform-azurerg.bicep"
			if custom {
				tenant, client := azureFederatedIdentity(ctx, t)
				require.NotEmpty(t, tenant, "custom provider test requires the workload identity tenant")
				require.NotEmpty(t, client, "custom provider test requires the workload identity client")
				template = "testdata/corerp-resources-terraform-azurerg-custom-provider.bicep"
				parameters = append(parameters, "tenantId="+tenant, "clientId="+client,
					"oidcTokenFilePath="+providers.AzureOIDCTokenFilePath)
			}

			// The driver omits Azure scope IDs such as resource groups from output resources,
			// so the Azure SDK checks below verify the recipe lifecycle instead.
			ct := rp.NewRPTest(t, name, []rp.TestStep{{
				Executor: step.NewDeployExecutor(template, parameters...),
				RPResources: &validation.RPResourceSet{Resources: []validation.RPResource{
					{Name: envName, Type: validation.EnvironmentsResource},
					{Name: appName, Type: validation.ApplicationsResource},
					{Name: name, Type: validation.ExtendersResource, App: appName},
				}},
				SkipObjectValidation: true, // This recipe creates no Kubernetes workload.
				PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
					resourceID := "/planes/radius/local/resourcegroups/kind-radius/providers/Applications.Core/extenders/" + name
					suffix, err := corerp.GetSecretSuffix(resourceID, envName, appName)
					require.NoError(t, err)
					secret, err := ct.Options.K8sClient.CoreV1().Secrets(secretNamespace).Get(ctx, secretPrefix+suffix, metav1.GetOptions{})
					require.NoError(t, err)
					require.NotEmpty(t, secret.Data, "Terraform state must remain available for destroy")

					group, err := groups.Get(ctx, groupName, nil)
					require.NoError(t, err)
					require.NotNil(t, group.ID)
					require.Equal(t, "/subscriptions/"+subscription+"/resourceGroups/"+groupName, *group.ID)
				},
			}})
			if custom {
				secretStoreName := name + "-provider-settings"
				ct.Steps[0].RPResources.Resources = append(ct.Steps[0].RPResources.Resources,
					validation.RPResource{Name: secretStoreName, Type: validation.SecretStoresResource})
				t.Cleanup(func() {
					cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
					defer cancel()
					_, err := ct.Options.ManagementClient.DeleteResource(cleanupCtx, validation.SecretStoresResource, secretStoreName, false)
					if err != nil && !isNotFoundResponse(err) {
						t.Errorf("delete provider settings secret store: %v", err)
					}
				})
			}
			ct.PostDeleteVerify = func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				resourceID := "/planes/radius/local/resourcegroups/kind-radius/providers/Applications.Core/extenders/" + name
				corerp.TestSecretDeletion(t, ctx, ct, appName, envName, resourceID, secretNamespace, secretPrefix)
				_, err := groups.Get(ctx, groupName, nil)
				require.True(t, isNotFoundResponse(err), "Terraform destroy must remove the Azure resource group: %v", err)
			}
			ct.FastCleanup = false
			ct.RunSerial = true
			ct.Test(t)
		})
	}
}

// Test_TerraformPrivateGitModule_KubernetesRedis covers the following terraform recipe scenario:
//
// - Create an extender resource using a Terraform recipe stored in private terraform git repository that deploys Redis on Kubernetes.
// - The recipe deployment creates a Kubernetes deployment and a Kubernetes service.
//
// This test uses a recipe stored in a private repository in radius-project organization and uses PAT from radius account, so it cannot be tested locally.
// To run this test locally:
// - Upload the files from test/testrecipes/test-terraform-recipes/kubernetes-redis/modules to a private repository and update the module source in testutil.GetTerraformPrivateModuleSource()
// - Create a PAT to access the private repository and update testutil.GetGitPAT() to return the generated PAT.
func Test_TerraformPrivateGitModule_KubernetesRedis(t *testing.T) {
	template := "testdata/corerp-resources-terraform-private-git-repo-redis.bicep"
	name := "corerp-resources-terraform-private-redis"
	appName := "corerp-resources-terraform-private-app"
	envName := "corerp-resources-terraform-private-env"
	redisCacheName := "tf-redis-cache-private"

	secretSuffix, err := corerp.GetSecretSuffix("/planes/radius/local/resourcegroups/kind-radius/providers/Applications.Core/extenders/"+name, envName, appName)
	require.NoError(t, err)
	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetTerraformPrivateModuleSource(), "appName="+appName, "redisCacheName="+redisCacheName, testutil.GetGitPAT()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: envName,
						Type: validation.EnvironmentsResource,
					},
					{
						Name: appName,
						Type: validation.ApplicationsResource,
					},
					{
						Name: name,
						Type: validation.ExtendersResource,
						App:  appName,
						OutputResources: []validation.OutputResourceResponse{
							{ID: "/planes/kubernetes/local/namespaces/corerp-resources-terraform-private-app/providers/apps/Deployment/tf-redis-cache-private"},
							{ID: "/planes/kubernetes/local/namespaces/corerp-resources-terraform-private-app/providers/core/Service/tf-redis-cache-private"},
						},
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appName: {
						validation.NewK8sServiceForResource(appName, redisCacheName).
							ValidateLabels(false),
					},
					secretNamespace: {
						validation.NewK8sSecretForResourceWithResourceName(secretPrefix + secretSuffix).
							ValidateLabels(false),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				secret, err := test.Options.K8sClient.CoreV1().Secrets(secretNamespace).
					Get(ctx, secretPrefix+secretSuffix, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, secretNamespace, secret.Namespace)
				require.Equal(t, secretPrefix+secretSuffix, secret.Name)

				redis, err := test.Options.ManagementClient.GetResource(ctx, "Applications.Core/extenders", name)
				require.NoError(t, err)
				require.NotNil(t, redis)
				status := redis.Properties["status"].(map[string]any)
				recipe := status["recipe"].(map[string]any)
				require.Equal(t, "terraform", recipe["templateKind"].(string))
				expectedTemplatePath := strings.Replace(testutil.GetTerraformPrivateModuleSource(), "privateGitModule=", "", 1)
				require.Equal(t, expectedTemplatePath, recipe["templatePath"].(string))
				// At present, it is not possible to verify the template version in functional tests
				// This is verified by UTs though

				// Manually delete Kubernetes the secret that stores the Terraform state file now. The next step in the test will be the deletion
				// of the portable resource that uses this secret for Terraform recipe. This is to verify that the test and portable resource
				// deletion will not fail even though the secret is already deleted.
				err = test.Options.K8sClient.CoreV1().Secrets(secretNamespace).Delete(ctx, secretPrefix+secretSuffix, metav1.DeleteOptions{})
				require.NoError(t, err)
			},
		},
	})

	test.PostDeleteVerify = func(ctx context.Context, t *testing.T, test rp.RPTest) {
		resourceID := "/planes/radius/local/resourcegroups/kind-radius/providers/Applications.Core/extenders/" + name
		corerp.TestSecretDeletion(t, ctx, test, appName, envName, resourceID, secretNamespace, secretPrefix)
	}

	test.Test(t)
}
