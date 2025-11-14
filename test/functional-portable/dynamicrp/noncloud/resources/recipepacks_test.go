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

import (
	"context"
	"strings"
	"testing"

	"github.com/radius-project/radius/test"
	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Test_RecipePacks_Deployment tests the deployment and functionality of Radius.Core/recipePacks resources.
// This test validates that recipe packs can be created with user-defined type recipes, associated with environments,
// and used to deploy resources with their configured recipes via the new Radius.Core/environments resource.
//
// The test consists of the following steps:
// 1. Create Kubernetes namespace since we expect this to be created by Ops
// 2. Resource Type Registration:
//   - Registers user-defined resource type "Test.Resources/userTypeAlpha"
//   - Verifies the registration by checking if the resource type is listed in the CLI output
//
// 3. Resource Deployment:
//   - Deploys a Bicep template that creates a recipe pack with userTypeAlpha recipe
//   - Creates a Radius.Core/environments resource that references the recipe pack
//   - Deploys RRT resources that use the recipe from the pack
//
// 4. Validation:
//   - Validates that the recipe pack and environment are created successfully
//   - Confirms that RRT resources are deployed using the recipes from the pack
func Test_RecipePacks_Deployment(t *testing.T) {
	template := "testdata/recipepacks-test.bicep"
	appName := "recipepacks-test-app"
	appNamespace := "recipepacks-ns"
	parentResourceTypeName := "Test.Resources/userTypeAlpha"
	parentResourceTypeParam := strings.Split(parentResourceTypeName, "/")[1]
	filepath := "testdata/testresourcetypes.yaml"
	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	test := rp.NewRPTest(t, appName, []rp.TestStep{
		{
			// The first step in this test is to create the Kubernetes namespace.
			Executor: step.NewFuncExecutor(func(ctx context.Context, t *testing.T, options test.TestOptions) {
				_, err := options.K8sClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: appNamespace,
					},
				}, metav1.CreateOptions{})
				if err != nil && !strings.Contains(err.Error(), "already exists") {
					require.NoError(t, err)
				}
			}),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				_, err := test.Options.K8sClient.CoreV1().Namespaces().Get(ctx, appNamespace, metav1.GetOptions{})
				require.NoError(t, err, "Namespace should be created")
			},
		},
		{
			// The second step in this test is to create/register the parent user-defined resource type using the CLI.
			Executor: step.NewFuncExecutor(func(ctx context.Context, t *testing.T, options test.TestOptions) {
				_, err := cli.ResourceTypeCreate(ctx, parentResourceTypeParam, filepath)
				require.NoError(t, err)
			}),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				output, err := cli.RunCommand(ctx, []string{"resource-type", "show", parentResourceTypeName, "--output", "json"})
				require.NoError(t, err)
				require.Contains(t, output, parentResourceTypeName)
			},
		},
		{
			// The third step is to deploy a bicep file using a recipe pack for the resource type registered.
			Executor:                               step.NewDeployExecutor(template, testutil.GetBicepRecipeRegistry(), testutil.GetBicepRecipeVersion()),
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   false,
			SkipKubernetesOutputResourceValidation: true,
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "test-recipe-pack",
						Type: "radius.core/recipepacks",
					},
					{
						Name: "recipepacks-test-env",
						Type: "radius.core/environments",
					},
					{
						Name: appName,
						Type: validation.ApplicationsResource,
						App:  appName,
					},
					{
						Name: "rrtresource",
						Type: "test.resources/usertypealpha",
						App:  appName,
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				// Verify deployments exist in the specified namespace
				deployments, err := test.Options.K8sClient.AppsV1().Deployments(appNamespace).List(ctx, metav1.ListOptions{})
				require.NoError(t, err)
				require.NotEmpty(t, deployments.Items, "No deployments found in namespace %s", appNamespace)

				t.Logf("Found %d deployments in namespace %s", len(deployments.Items), appNamespace)
				for _, deploy := range deployments.Items {
					t.Logf("Deployment: %s", deploy.Name)
				}

				// Clean up the namespace after verification
				err = test.Options.K8sClient.CoreV1().Namespaces().Delete(ctx, appNamespace, metav1.DeleteOptions{})
				if err != nil {
					t.Logf("Warning: Failed to delete namespace %s: %v", appNamespace, err)
				} else {
					t.Logf("Successfully deleted namespace %s", appNamespace)
				}
			},
		},
	})

	test.Test(t)
}

// Test_RecipePacks_NoProvider_Failure tests that deployment fails when Radius.Core/environments
// does not include a providers.kubernetes.namespace configuration.
// This test validates that the system properly enforces namespace requirements.
//
// The test consists of the following steps:
// 1. Resource Type Registration:
//   - Registers user-defined resource type "Test.Resources/userTypeAlpha"
//
// 2. Resource Deployment Failure:
//   - Attempts to deploy a Bicep template with a recipe pack but no providers configuration in the environment
//   - The recipe does not have a default namspace
//   - Validates that the deployment fails with "Namespace parameter required." error
func Test_RecipePacks_NoProvider_Failure(t *testing.T) {
	template := "testdata/recipepacks-test-no-provider.bicep"
	appName := "recipepacks-test-app-no-provider"
	parentResourceTypeName := "Test.Resources/userTypeAlpha"
	parentResourceTypeParam := strings.Split(parentResourceTypeName, "/")[1]
	filepath := "testdata/testresourcetypes.yaml"
	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	validate := step.ValidateSingleDetail("DeploymentFailed", step.DeploymentErrorDetail{
		Code: "ResourceDeploymentFailure",
		Details: []step.DeploymentErrorDetail{
			{
				Code:            "RecipeDeploymentFailed",
				MessageContains: "failed to deploy recipe default of type Test.Resources/userTypeAlpha",
				Details: []step.DeploymentErrorDetail{
					{
						Code:            "DeploymentFailed",
						MessageContains: "At least one resource deployment operation failed",
						Details: []step.DeploymentErrorDetail{
							{
								Code:            "",
								MessageContains: "Namespace parameter required.",
							},
						},
					},
				},
			},
		},
	})

	test := rp.NewRPTest(t, appName, []rp.TestStep{
		{
			// The first step in this test is to create/register the parent user-defined resource type using the CLI.
			Executor: step.NewFuncExecutor(func(ctx context.Context, t *testing.T, options test.TestOptions) {
				_, err := cli.ResourceTypeCreate(ctx, parentResourceTypeParam, filepath)
				require.NoError(t, err)
			}),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				output, err := cli.RunCommand(ctx, []string{"resource-type", "show", parentResourceTypeName, "--output", "json"})
				require.NoError(t, err)
				require.Contains(t, output, parentResourceTypeName)
			},
		},
		{
			// The second step is to deploy a bicep file without providers configuration - this should fail
			Executor:                               step.NewDeployErrorExecutor(template, validate, testutil.GetBicepRecipeRegistry(), testutil.GetBicepRecipeVersion()),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   false,
		},
	})

	test.Test(t)
}

// Test_RecipePacks_MissingNamespace_Failure tests that deployment fails when the namespace specified
// in the Radius.Core/environments providers.kubernetes.namespace does not exist in the cluster.
// This test validates that the system properly checks for namespace existence before deployment.
//
// The test consists of the following steps:
// 1. Resource Type Registration (no namespace creation):
//   - Registers user-defined resource type "Test.Resources/userTypeAlpha"
//
// 2. Resource Deployment Failure:
//   - Attempts to deploy a Bicep template that references a non-existent namespace
//   - Validates that the deployment fails with "Namespace 'recipepacks-ns' does not exist" error
func Test_RecipePacks_MissingNamespace_Failure(t *testing.T) {
	template := "testdata/recipepacks-test.bicep"
	appName := "recipepacks-test-app"
	parentResourceTypeName := "Test.Resources/userTypeAlpha"
	parentResourceTypeParam := strings.Split(parentResourceTypeName, "/")[1]
	filepath := "testdata/testresourcetypes.yaml"
	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	validate := step.ValidateSingleDetail("DeploymentFailed", step.DeploymentErrorDetail{
		Code:            "BadRequest",
		MessageContains: "Namespace 'recipepacks-ns' does not exist in the Kubernetes cluster. Please create it before proceeding.",
	})

	test := rp.NewRPTest(t, appName, []rp.TestStep{
		{
			// The first step in this test is to create/register the parent user-defined resource type using the CLI.
			// NOTE: We deliberately skip namespace creation to test the failure case
			Executor: step.NewFuncExecutor(func(ctx context.Context, t *testing.T, options test.TestOptions) {
				_, err := cli.ResourceTypeCreate(ctx, parentResourceTypeParam, filepath)
				require.NoError(t, err)
			}),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				output, err := cli.RunCommand(ctx, []string{"resource-type", "show", parentResourceTypeName, "--output", "json"})
				require.NoError(t, err)
				require.Contains(t, output, parentResourceTypeName)
			},
		},
		{
			// The second step is to deploy a bicep file with a non-existent namespace - this should fail
			Executor:                               step.NewDeployErrorExecutor(template, validate, testutil.GetBicepRecipeRegistry(), testutil.GetBicepRecipeVersion()),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   false,
		},
	})

	test.Test(t)
}
