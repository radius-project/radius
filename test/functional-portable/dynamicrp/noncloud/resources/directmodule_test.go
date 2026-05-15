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

// Test_DirectModule_Terraform tests deployment of a direct (non-wrapped) Terraform module
// via recipe packs. This validates:
// - {{context.resource.name}} and {{context.runtime.kubernetes.namespace}} expression resolution
// - outputs mapping from module output names to resource property names
// - Terraform module download and execution via HTTP archive source
//
// Steps:
// 1. Create Kubernetes namespace
// 2. Register user-defined resource type
// 3. Deploy Bicep template with direct module recipe pack (outputs mapping + expression params)
// 4. Verify Kubernetes deployments are created in the target namespace
func Test_DirectModule_Terraform(t *testing.T) {
	template := "testdata/directmodule-tf-test.bicep"
	appName := "directmodule-tf-app"
	appNamespace := "directmodule-tf-ns"
	parentResourceTypeName := "Test.Resources/userTypeAlpha"
	parentResourceTypeParam := strings.Split(parentResourceTypeName, "/")[1]
	filepath := "testdata/testresourcetypes.yaml"
	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	test := rp.NewRPTest(t, appName, []rp.TestStep{
		{
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
		},
		{
			Executor: step.NewFuncExecutor(func(ctx context.Context, t *testing.T, options test.TestOptions) {
				_, err := cli.ResourceTypeCreate(ctx, parentResourceTypeParam, filepath)
				require.NoError(t, err)
			}),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
		},
		{
			Executor:                               step.NewDeployExecutor(template, testutil.GetTerraformRecipeModuleServerURL(), "appName="+appName),
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   false,
			SkipKubernetesOutputResourceValidation: true,
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "directmodule-tf-recipe-pack",
						Type: "radius.core/recipepacks",
					},
					{
						Name: "directmodule-tf-env",
						Type: "radius.core/environments",
					},
					{
						Name: appName,
						Type: validation.ApplicationsResource,
						App:  appName,
					},
					{
						Name: "directmodule-tf-resource",
						Type: "test.resources/usertypealpha",
						App:  appName,
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				// Verify that Terraform created Kubernetes deployments in the target namespace.
				// The direct module should have resolved {{context.runtime.kubernetes.namespace}}
				// to the namespace configured in the environment's providers.
				deployments, err := test.Options.K8sClient.AppsV1().Deployments(appNamespace).List(ctx, metav1.ListOptions{})
				require.NoError(t, err)
				require.NotEmpty(t, deployments.Items, "Expected Terraform module to create deployments in namespace %s", appNamespace)

				t.Logf("Found %d deployments in namespace %s", len(deployments.Items), appNamespace)
				for _, deploy := range deployments.Items {
					t.Logf("  Deployment: %s", deploy.Name)
				}

				// Clean up namespace
				err = test.Options.K8sClient.CoreV1().Namespaces().Delete(ctx, appNamespace, metav1.DeleteOptions{})
				if err != nil {
					t.Logf("Warning: Failed to delete namespace %s: %v", appNamespace, err)
				}
			},
		},
	})

	test.Test(t)
}

// Test_DirectModule_BackwardCompat tests that traditional wrapped recipes (with context variable
// and result output) continue to work correctly after the direct module support changes.
// This is a regression test to ensure no behavioral changes for existing recipe patterns.
//
// Steps:
// 1. Create Kubernetes namespace
// 2. Register user-defined resource type
// 3. Deploy Bicep template with wrapped recipe (existing pattern)
// 4. Verify deployment succeeds and Kubernetes resources are created
// 5. Verify recipe parameter reconciliation (environment overrides recipe pack params)
func Test_DirectModule_BackwardCompat(t *testing.T) {
	template := "testdata/directmodule-compat-test.bicep"
	appName := "directmodule-compat-app"
	appNamespace := "directmodule-compat-ns"
	parentResourceTypeName := "Test.Resources/userTypeAlpha"
	parentResourceTypeParam := strings.Split(parentResourceTypeName, "/")[1]
	filepath := "testdata/testresourcetypes.yaml"
	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	test := rp.NewRPTest(t, appName, []rp.TestStep{
		{
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
		},
		{
			Executor: step.NewFuncExecutor(func(ctx context.Context, t *testing.T, options test.TestOptions) {
				_, err := cli.ResourceTypeCreate(ctx, parentResourceTypeParam, filepath)
				require.NoError(t, err)
			}),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
		},
		{
			Executor:                               step.NewDeployExecutor(template, testutil.GetBicepRecipeRegistry(), testutil.GetBicepRecipeVersion(), "appName="+appName),
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   false,
			SkipKubernetesOutputResourceValidation: true,
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "directmodule-compat-recipe-pack",
						Type: "radius.core/recipepacks",
					},
					{
						Name: "directmodule-compat-env",
						Type: "radius.core/environments",
					},
					{
						Name: appName,
						Type: validation.ApplicationsResource,
						App:  appName,
					},
					{
						Name: "directmodule-compat-resource",
						Type: "test.resources/usertypealpha",
						App:  appName,
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				// Verify that the wrapped recipe deployed Kubernetes resources.
				deployments, err := test.Options.K8sClient.AppsV1().Deployments(appNamespace).List(ctx, metav1.ListOptions{})
				require.NoError(t, err)
				require.NotEmpty(t, deployments.Items, "Expected wrapped recipe to create deployments in namespace %s", appNamespace)

				t.Logf("Found %d deployments in namespace %s", len(deployments.Items), appNamespace)

				// Verify recipe parameter reconciliation: environment sets port=9090, recipe pack sets port=8080.
				// Environment-level parameters should override recipe pack parameters.
				foundPort := false
				for _, deploy := range deployments.Items {
					for _, container := range deploy.Spec.Template.Spec.Containers {
						for _, port := range container.Ports {
							if port.ContainerPort == 9090 {
								foundPort = true
								t.Logf("  ✓ Container %s has expected port 9090 (env override)", container.Name)
							}
						}
					}
				}
				require.True(t, foundPort, "Expected container with port 9090 (environment parameter should override recipe pack parameter)")

				// Clean up namespace
				err = test.Options.K8sClient.CoreV1().Namespaces().Delete(ctx, appNamespace, metav1.DeleteOptions{})
				if err != nil {
					t.Logf("Warning: Failed to delete namespace %s: %v", appNamespace, err)
				}
			},
		},
	})

	test.Test(t)
}
