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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_radius "github.com/radius-project/radius/pkg/ucp/resources/radius"
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

// Test_TerraformRecipe_Redis covers the following terraform recipe scenario:
//
// - Create an extender resource using a Terraform recipe that deploys Redis on Kubernetes.
// - The recipe deployment creates a Kubernetes deployment and a Kubernetes service.
func Test_TerraformRecipe_KubernetesRedis(t *testing.T) {
	template := "testdata/corerp-resources-terraform-redis.bicep"
	name := "corerp-resources-terraform-redis"
	appName := "corerp-resources-terraform-redis-app"
	envName := "corerp-resources-terraform-redis-env"
	redisCacheName := "tf-redis-cache"

	secretSuffix, err := corerp.GetSecretSuffix("/planes/radius/local/resourcegroups/kind-radius/providers/Applications.Core/extenders/"+name, envName, appName)
	require.NoError(t, err)

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetTerraformRecipeModuleServerURL(), "appName="+appName, "redisCacheName="+redisCacheName),
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
							{ID: "/planes/kubernetes/local/namespaces/corerp-resources-terraform-redis-app/providers/apps/Deployment/tf-redis-cache"},
							{ID: "/planes/kubernetes/local/namespaces/corerp-resources-terraform-redis-app/providers/core/Service/tf-redis-cache"},
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
				recipe := status["recipe"].(map[string]interface{})
				require.Equal(t, "terraform", recipe["templateKind"].(string))
				expectedTemplatePath := strings.Replace(testutil.GetTerraformRecipeModuleServerURL()+"/kubernetes-redis.zip//modules", "moduleServer=", "", 1)
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

// Test_TerraformRecipe_KubernetesPostgres covers the following terraform recipe scenario:
//
// - Create an extender resource using a Terraform recipe that deploys Postgres on Kubernetes.
// - The recipe deployment creates a Kubernetes deployment and a Kubernetes service and a postgres db.
func Test_TerraformRecipe_KubernetesPostgres(t *testing.T) {
	template := "testdata/corerp-resources-terraform-postgres.bicep"
	appName := "corerp-resources-terraform-pg-app"
	envName := "corerp-resources-terraform-pg-env"
	extenderName := "pgs-resources-terraform-pgsapp"
	secretName := "pgs-secretstore"
	secretResourceName := appName + "/" + secretName
	userName := "postgres"
	password := "abc-123-hgd-@#$'"

	secretSuffix, err := corerp.GetSecretSuffix("/planes/radius/local/resourcegroups/kind-radius/providers/Applications.Core/extenders/"+extenderName, envName, appName)
	require.NoError(t, err)

	test := rp.NewRPTest(t, appName, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetTerraformRecipeModuleServerURL(), "userName="+userName, "password="+password),
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
						Name: extenderName,
						Type: validation.ExtendersResource,
						App:  appName,
						OutputResources: []validation.OutputResourceResponse{
							{ID: "/planes/kubernetes/local/namespaces/corerp-resources-terraform-pg-app/providers/apps/Deployment/postgres"},
							{ID: "/planes/kubernetes/local/namespaces/corerp-resources-terraform-pg-app/providers/core/Service/postgres"},
						},
					},
					{
						Name: secretName,
						Type: validation.SecretStoresResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appName: {
						validation.NewK8sServiceForResource(appName, "postgres").
							ValidateLabels(false),
						validation.NewK8sPodForResource(appName, "postgres").
							ValidateLabels(false),
						validation.NewK8sSecretForResourceWithResourceName(secretResourceName),
					},
					secretNamespace: {
						validation.NewK8sSecretForResourceWithResourceName(secretPrefix + secretSuffix).
							ValidateLabels(false),
					},
				},
			},
			SkipObjectValidation: true,
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				secret, err := test.Options.K8sClient.CoreV1().Secrets(secretNamespace).
					Get(ctx, secretPrefix+secretSuffix, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, secretNamespace, secret.Namespace)
				require.Equal(t, secretPrefix+secretSuffix, secret.Name)

				pg, err := test.Options.ManagementClient.GetResource(ctx, "Applications.Core/extenders", extenderName)
				require.NoError(t, err)
				require.NotNil(t, pg)
				status := pg.Properties["status"].(map[string]any)
				recipe := status["recipe"].(map[string]interface{})
				require.Equal(t, "terraform", recipe["templateKind"].(string))
				expectedTemplatePath := strings.Replace(testutil.GetTerraformRecipeModuleServerURL()+"/postgres.zip", "moduleServer=", "", 1)
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
		resourceID := "/planes/radius/local/resourcegroups/kind-radius/providers/Applications.Core/extenders/" + extenderName
		corerp.TestSecretDeletion(t, ctx, test, appName, envName, resourceID, secretNamespace, secretPrefix)
	}

	test.Test(t)
}

func Test_TerraformRecipe_Context(t *testing.T) {
	template := "testdata/corerp-resources-terraform-context.bicep"
	name := "corerp-resources-terraform-context"
	appNamespace := "corerp-resources-terraform-context-app"

	secretSuffix, err := corerp.GetSecretSuffix("/planes/radius/local/resourcegroups/kind-radius/providers/Applications.Core/extenders/"+name, name, name)
	require.NoError(t, err)

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetTerraformRecipeModuleServerURL()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: name,
						Type: validation.ExtendersResource,
						App:  name,
						OutputResources: []validation.OutputResourceResponse{
							{ID: "/planes/kubernetes/local/namespaces/corerp-resources-terraform-context-app/providers/core/Secret/corerp-resources-terraform-context"},
						},
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sSecretForResource(name, name),
					},
					secretNamespace: {
						validation.NewK8sSecretForResourceWithResourceName(secretPrefix + secretSuffix).
							ValidateLabels(false),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				// `k8ssecret-context` recipe should have created a secret with the populated recipe context.
				s, err := test.Options.K8sClient.CoreV1().Secrets(appNamespace).Get(ctx, name, metav1.GetOptions{})
				require.NoError(t, err)

				decoded, err := base64.StdEncoding.DecodeString(string(s.Data["resource.id"]))
				require.NoError(t, err)
				r, err := resources.ParseResource(string(decoded))
				require.NoError(t, err)

				rgName := r.FindScope(resources_radius.ScopeResourceGroups)

				tests := []struct {
					key      string
					expected string
				}{
					{
						key:      "resource.type",
						expected: "Applications.Core/extenders",
					},
					{
						key:      "azure.subscription_id",
						expected: "00000000-0000-0000-0000-100000000000",
					},
					{
						key:      "recipe_context",
						expected: "{\"application\":{\"id\":\"/planes/radius/local/resourcegroups/radiusGroup/providers/Applications.Core/applications/corerp-resources-terraform-context\",\"name\":\"corerp-resources-terraform-context\"},\"aws\":null,\"azure\":{\"resourceGroup\":{\"id\":\"/subscriptions/00000000-0000-0000-0000-100000000000/resourceGroups/rg-terraform-context\",\"name\":\"rg-terraform-context\"},\"subscription\":{\"id\":\"/subscriptions/00000000-0000-0000-0000-100000000000\",\"subscriptionId\":\"00000000-0000-0000-0000-100000000000\"}},\"environment\":{\"id\":\"/planes/radius/local/resourcegroups/radiusGroup/providers/Applications.Core/environments/corerp-resources-terraform-context\",\"name\":\"corerp-resources-terraform-context\"},\"resource\":{\"id\":\"/planes/radius/local/resourcegroups/radiusGroup/providers/Applications.Core/extenders/corerp-resources-terraform-context\",\"name\":\"corerp-resources-terraform-context\",\"type\":\"Applications.Core/extenders\"},\"runtime\":{\"kubernetes\":{\"environmentNamespace\":\"corerp-resources-terraform-context-env\",\"namespace\":\"corerp-resources-terraform-context-app\"}}}",
					},
				}

				for _, tc := range tests {
					decoded, err := base64.StdEncoding.DecodeString(string(s.Data[tc.key]))
					require.NoErrorf(t, err, "failed to decode secret data, key: %s", tc.key)
					// Replace the resource group name with a fake name because resourcegroup can be changed by test setup.
					replaced := strings.ReplaceAll(string(decoded), "resourcegroups/"+rgName, "resourcegroups/radiusGroup")
					require.Equalf(t, tc.expected, replaced, "secret data mismatch, key: %s", tc.key)
				}

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
		corerp.TestSecretDeletion(t, ctx, test, name, name, resourceID, secretNamespace, secretPrefix)
	}

	test.Test(t)
}

// Test_TerraformRecipe_ParametersAndOutputs Validates input parameters correctly set and output values/secrets are populated.
func Test_TerraformRecipe_ParametersAndOutputs(t *testing.T) {
	template := "testdata/corerp-resources-terraform-recipe-terraform.bicep"
	name := "corerp-resources-terraform-parametersandoutputs"

	// Best way to pass complex parameters is to use JSON.
	parametersFilePath := testutil.WriteBicepParameterFile(t, map[string]any{
		// These will be set on the environment as part of the recipe
		"environmentParameters": map[string]any{
			"a": "environment",
			"d": "environment",
		},

		// These will be set on the extender resource
		"resourceParameters": map[string]any{
			"c": 42,
			"d": "resource",
		},
	})

	parameters := []string{
		testutil.GetTerraformRecipeModuleServerURL(),
		fmt.Sprintf("basename=%s", name),
		fmt.Sprintf("moduleName=%s", "parameter-outputs"),
		"@" + parametersFilePath,
	}

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, parameters...),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.EnvironmentsResource,
					},
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: name,
						Type: validation.ExtendersResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{},
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				resource, err := test.Options.ManagementClient.GetResource(ctx, "Applications.Core/extenders", name)
				require.NoError(t, err)

				text, err := json.MarshalIndent(resource, "", "  ")
				require.NoError(t, err)
				t.Logf("resource data:\n %s", text)

				require.Equal(t, "environment", resource.Properties["a"])
				require.Equal(t, "default value", resource.Properties["b"])
				require.Equal(t, 42.0, resource.Properties["c"])
				require.Equal(t, "resource", resource.Properties["d"])

				response, err := test.Options.CustomAction.InvokeCustomAction(ctx, *resource.ID, "2023-10-01-preview", "listSecrets")
				require.NoError(t, err)

				expected := map[string]any{"e": "secret value"}
				require.Equal(t, expected, response.Body)
			},
		},
	})
	test.Test(t)
}

// Test_TerraformRecipe_WrongOutput validates that a Terraform recipe with invalid "result" output schema returns an error.
func Test_TerraformRecipe_WrongOutput(t *testing.T) {
	template := "testdata/corerp-resources-terraform-recipe-terraform.bicep"
	name := "corerp-resources-terraform-wrong-output"

	parameters := []string{
		testutil.GetTerraformRecipeModuleServerURL(),
		fmt.Sprintf("basename=%s", name),
		fmt.Sprintf("moduleName=%s", "wrong-output"),
	}

	validate := step.ValidateSingleDetail("DeploymentFailed", step.DeploymentErrorDetail{
		Code: "ResourceDeploymentFailure",
		Details: []step.DeploymentErrorDetail{
			{
				Code:            recipes.InvalidRecipeOutputs,
				MessageContains: "failed to read the recipe output \"result\": json: unknown field \"error\"",
			},
		},
	})

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployErrorExecutor(template, validate, parameters...),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.EnvironmentsResource,
					},
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
				},
			},
			SkipObjectValidation: true,
		},
	})

	test.Test(t)
}
