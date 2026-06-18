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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Test_DirectModule_Terraform tests deployment of a direct (non-wrapped) Terraform module
// via recipe packs. The module under test
// (test/testrecipes/test-terraform-recipes/direct-kubernetes) has no `context` input
// variable and no structured `result` output, so it exercises the direct-module code path.
//
// This validates:
//   - {{context.resource.name}} and {{context.runtime.kubernetes.namespace}} expression resolution
//   - outputs mapping from module output names to resource property names
//   - Terraform module download and execution via an HTTP archive source
//
// Steps:
//  1. Create the Kubernetes namespace (expected to be created by Ops).
//  2. Register the user-defined resource type "Test.Resources/userTypeAlpha".
//  3. Deploy a Bicep template with a direct-module recipe pack (outputs mapping + expression params).
//  4. Verify Kubernetes deployments are created in the target namespace, proving the namespace
//     expression resolved and the module executed.
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
			// Create the Kubernetes namespace the recipe deploys into.
			Executor: step.NewFuncExecutor(func(ctx context.Context, t *testing.T, options test.TestOptions) {
				_, err := options.K8sClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: appNamespace,
					},
				}, metav1.CreateOptions{})
				if err != nil && !apierrors.IsAlreadyExists(err) {
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
			// Register the user-defined resource type using the CLI.
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
			// Deploy the direct-module recipe pack and the resource that uses it.
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
				// The direct module resolves {{context.runtime.kubernetes.namespace}} to the
				// namespace configured in the environment's providers, proving expression
				// resolution and module execution end-to-end.
				deployments, err := test.Options.K8sClient.AppsV1().Deployments(appNamespace).List(ctx, metav1.ListOptions{})
				require.NoError(t, err)
				require.NotEmpty(t, deployments.Items, "Expected Terraform module to create deployments in namespace %s", appNamespace)

				t.Logf("Found %d deployments in namespace %s", len(deployments.Items), appNamespace)
				for _, deploy := range deployments.Items {
					t.Logf("  Deployment: %s", deploy.Name)
				}
			},
		},
	})

	// Delete the namespace created in the first step. This runs in the test-level
	// PostDeleteVerify, which executes after the RP cleanup phase (Terraform destroy), so the
	// namespace is not torn down while an in-flight destroy still needs it.
	test.PostDeleteVerify = func(ctx context.Context, t *testing.T, ct rp.RPTest) {
		err := ct.Options.K8sClient.CoreV1().Namespaces().Delete(ctx, appNamespace, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			t.Logf("Warning: Failed to delete namespace %s: %v", appNamespace, err)
		}
	}

	test.Test(t)
}
