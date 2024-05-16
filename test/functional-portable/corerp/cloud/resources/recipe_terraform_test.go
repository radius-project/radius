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
// be consistent across all resource types. These tests mostly use the extender resource type and mostly
// avoid cloud resources to avoid unnecessary coupling and reliability issues.
//
// Tests in this file should only use cloud resources if absolutely necessary.
//
// Tests in this file should be kept *roughly* in sync with recipe_bicep_test and any other drivers.

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/radius-project/radius/test/functional-portable/corerp"
	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
)

var (
	secretNamespace = "radius-system"
	secretPrefix    = "tfstate-default-"
)

// Test_TerraformRecipe_AzureStorage creates an Extender resource consuming a Terraform recipe that deploys an Azure blob storage instance.
func Test_TerraformRecipe_AzureStorage(t *testing.T) {
	template := "testdata/corerp-resources-terraform-azurestorage.bicep"
	name := "corerp-resources-terraform-azstorage"
	appName := "corerp-resources-terraform-azstorage-app"
	envName := "corerp-resources-terraform-azstorage-env"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetTerraformRecipeModuleServerURL(), "appName="+appName),
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
					},
				},
			},
			SkipObjectValidation: true,
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				resourceID := "/planes/radius/local/resourcegroups/kind-radius/providers/Applications.Core/extenders/" + name
				secretSuffix, err := corerp.GetSecretSuffix(resourceID, envName, appName)
				require.NoError(t, err)

				secret, err := test.Options.K8sClient.CoreV1().Secrets(secretNamespace).
					Get(ctx, secretPrefix+secretSuffix, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, secretNamespace, secret.Namespace)
				require.Equal(t, secretPrefix+secretSuffix, secret.Name)
			},
		},
	})

	test.PostDeleteVerify = func(ctx context.Context, t *testing.T, test rp.RPTest) {
		resourceID := "/planes/radius/local/resourcegroups/kind-radius/providers/Applications.Core/extenders/" + name
		corerp.TestSecretDeletion(t, ctx, test, appName, envName, resourceID, secretNamespace, secretPrefix)
	}

	test.Test(t)
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
				// Test Recipe Show
				t.Run("Validate rad recipe show - terraform recipe", func(t *testing.T) {
					options := rp.NewRPTestOptions(t)
					cli := radcli.NewCLI(t, options.ConfigFilePath)
					output, err := cli.RecipeShow(ctx, envName, "default", "Applications.Datastores/redisCaches")
					require.NoError(t, err)
					require.Contains(t, output, "default")
					require.Contains(t, output, testutil.GetTerraformPrivateModuleSource())
					require.Contains(t, output, "Applications.Core/extenders")
					require.Contains(t, output, "redis_cache_name")
					require.Contains(t, output, "string")
				})
				secret, err := test.Options.K8sClient.CoreV1().Secrets(secretNamespace).
					Get(ctx, secretPrefix+secretSuffix, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, secretNamespace, secret.Namespace)
				require.Equal(t, secretPrefix+secretSuffix, secret.Name)

				redis, err := test.Options.ManagementClient.GetResource(ctx, "Applications.Core/extenders", name)
				require.NoError(t, err)
				require.NotNil(t, redis)
				status := redis.Properties["status"].(map[string]any)
				recipe := status["recipe"].(map[string]interface{})
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
