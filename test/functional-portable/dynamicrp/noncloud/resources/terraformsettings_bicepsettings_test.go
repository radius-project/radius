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
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Test_TerraformSettings_Redis tests that a Radius.Core/terraformSettingss resource can be created
// and referenced by an environment to provide Terraform recipe configuration (env vars).
// This test deploys a Terraform recipe (Redis on Kubernetes) via the new config resource path.
func Test_TerraformSettings_Redis(t *testing.T) {
	template := "testdata/terraformsettings-redis-test.bicep"
	appName := "tfconfig-redis-app"
	appNamespace := "tfconfig-redis-ns"

	test := rp.NewRPTest(t, appName, []rp.TestStep{
		{
			// Create the Kubernetes namespace (Radius.Core requires pre-existing namespaces).
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
			// Deploy the template with terraformSettings, recipePack, environment, and extender.
			Executor:                               step.NewDeployExecutor(template, testutil.GetTerraformRecipeModuleServerURL(), "appName="+appName),
			SkipObjectValidation:                   true,
			SkipKubernetesOutputResourceValidation: true,
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "test-terraform-config",
						Type: "radius.core/terraformsettings",
					},
					{
						Name: "tfconfig-recipe-pack",
						Type: "radius.core/recipepacks",
					},
					{
						Name: "tfconfig-redis-env",
						Type: "radius.core/environments",
					},
					{
						Name: appName,
						Type: validation.ApplicationsResource,
						App:  appName,
					},
					{
						Name: "tfconfig-redis-extender",
						Type: validation.ExtendersResource,
						App:  appName,
					},
				},
			},
		},
	})
	test.Test(t)
}

// Test_BicepSettings_CRUD tests that a Radius.Core/bicepSettingss resource can be created
// and referenced by an environment. This validates the CRUD path and environment wiring
// without requiring a private registry.
func Test_BicepSettings_CRUD(t *testing.T) {
	template := "testdata/bicepsettings-test.bicep"
	appName := "bicepsettings-test-app"
	appNamespace := "bicepsettings-test-ns"

	test := rp.NewRPTest(t, appName, []rp.TestStep{
		{
			// Create the Kubernetes namespace.
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
			// Deploy the template with bicepSettings, environment, and application.
			Executor:                               step.NewDeployExecutor(template, "appName="+appName),
			SkipObjectValidation:                   true,
			SkipKubernetesOutputResourceValidation: true,
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "bicepsettings-test-secret",
						Type: validation.SecretStoresResource,
					},
					{
						Name: "test-bicep-config",
						Type: "radius.core/bicepsettings",
					},
					{
						Name: "bicepsettings-test-env",
						Type: "radius.core/environments",
					},
					{
						Name: appName,
						Type: validation.ApplicationsResource,
						App:  appName,
					},
				},
			},
		},
	})
	test.Test(t)
}

// Test_TerraformSettings_SecuritySecret_Credentials regression-tests issue #12122: a
// Radius.Core/terraformSettingss resource must accept a Radius.Security/secrets reference for
// registry credentials, not only Applications.Core/secretStores.
//
// The test provisions a Radius.Security/secrets resource (whose Kubernetes recipe materializes a
// backing Kubernetes Secret holding a 'token' key) into a preview environment, then references that
// secret from a terraformSettings consumed by a separate Radius.Core environment. Deploying a Terraform
// recipe (Redis) through that environment forces the secret loader to resolve the credential from the
// Radius.Security/secrets resource: it reads the secret resource, locates its backing Kubernetes Secret
// output resource, and reads the 'token' value. Before the fix, this path rejected the
// Radius.Security/secrets type and recipe setup failed.
func Test_TerraformSettings_SecuritySecret_Credentials(t *testing.T) {
	template := "testdata/terraformsettings-securitysecret-test.bicep"
	appName := "tfsec-redis-app"
	appNamespace := "tfsec-redis-ns"
	secretsNamespace := "tfsec-secrets-ns"

	test := rp.NewRPTest(t, appName, []rp.TestStep{
		{
			// Create the consumer environment's Kubernetes namespace (Radius.Core requires pre-existing namespaces).
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
			// Placeholder executor; replaced below once the preview environment ID is known.
			Executor:                               step.NewDeployExecutor(template, testutil.GetTerraformRecipeModuleServerURL(), "appName="+appName),
			SkipObjectValidation:                   true,
			SkipKubernetesOutputResourceValidation: true,
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "tfsec-registry-token",
						Type: validation.SecuritySecretsResource,
					},
					{
						Name: "tfsec-terraform-config",
						Type: "radius.core/terraformsettings",
					},
					{
						Name: "tfsec-recipe-pack",
						Type: "radius.core/recipepacks",
					},
					{
						Name: "tfsec-redis-env",
						Type: "radius.core/environments",
					},
					{
						Name: appName,
						Type: validation.ApplicationsResource,
						App:  appName,
					},
					{
						Name: "tfsec-redis-extender",
						Type: validation.ExtendersResource,
						App:  appName,
					},
				},
			},
		},
	})

	// The Radius.Security/secrets recipe is registered in the preview environment's default recipe
	// pack, so provision the secret there. Its backing Kubernetes Secret lands in the preview namespace.
	preSetup, secretsEnvID := rp.NewPreviewEnvPreSetup("tfsec-secrets", test.Options.Workspace.Scope, secretsNamespace)
	test.PreSetup = preSetup
	test.Steps[1].Executor = step.NewDeployExecutor(template, testutil.GetTerraformRecipeModuleServerURL(), "appName="+appName, "secretsEnvironment="+secretsEnvID)

	test.Test(t)
}

// Test_TerraformSettings_BicepSettings_Combined deploys an environment that references
// both a Radius.Core/terraformSettingss resource (with terraformrc.providerInstallation
// and env vars) and a Radius.Core/bicepSettingss resource. It runs a Terraform recipe
// end-to-end to prove that:
//
//  1. The environment controller validates and resolves both config refs.
//  2. The recipe driver renders a .terraformrc from providerInstallation and points
//     Terraform at it via TF_CLI_CONFIG_FILE without breaking provider resolution.
//  3. Env vars from terraformSettings.env are propagated into the Terraform process.
//
// The test uses providerInstallation.direct (the default behavior — fetch all
// providers from the registry) so it does not require a network mirror in the
// test cluster.
func Test_TerraformSettings_BicepSettings_Combined(t *testing.T) {
	template := "testdata/tfbicep-combined-test.bicep"
	appName := "tfbicep-combined-app"
	appNamespace := "tfbicep-combined-ns"

	test := rp.NewRPTest(t, appName, []rp.TestStep{
		{
			// Create the Kubernetes namespace (Radius.Core requires pre-existing namespaces).
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
			// Deploy: terraformSettings + bicepSettings + recipePack + environment + app + extender (Terraform recipe).
			Executor:                               step.NewDeployExecutor(template, testutil.GetTerraformRecipeModuleServerURL(), "appName="+appName),
			SkipObjectValidation:                   true,
			SkipKubernetesOutputResourceValidation: true,
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "tfbicep-combined-registry-secret",
						Type: validation.SecretStoresResource,
					},
					{
						Name: "tfbicep-combined-tfconfig",
						Type: "radius.core/terraformsettings",
					},
					{
						Name: "tfbicep-combined-bicepsettings",
						Type: "radius.core/bicepsettings",
					},
					{
						Name: "tfbicep-combined-recipe-pack",
						Type: "radius.core/recipepacks",
					},
					{
						Name: "tfbicep-combined-env",
						Type: "radius.core/environments",
					},
					{
						Name: appName,
						Type: validation.ApplicationsResource,
						App:  appName,
					},
					{
						Name: "tfbicep-combined-extender",
						Type: validation.ExtendersResource,
						App:  appName,
					},
				},
			},
		},
	})
	test.Test(t)
}
