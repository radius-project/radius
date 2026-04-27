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

// Test_TerraformConfig_Redis tests that a Radius.Core/terraformConfigs resource can be created
// and referenced by an environment to provide Terraform recipe configuration (env vars).
// This test deploys a Terraform recipe (Redis on Kubernetes) via the new config resource path.
func Test_TerraformConfig_Redis(t *testing.T) {
	template := "testdata/terraformconfig-redis-test.bicep"
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
			// Deploy the template with terraformConfig, recipePack, environment, and extender.
			Executor:                               step.NewDeployExecutor(template, testutil.GetTerraformRecipeModuleServerURL(), "appName="+appName),
			SkipObjectValidation:                   true,
			SkipKubernetesOutputResourceValidation: true,
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "test-terraform-config",
						Type: "radius.core/terraformconfigs",
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

// Test_BicepConfig_CRUD tests that a Radius.Core/bicepConfigs resource can be created
// and referenced by an environment. This validates the CRUD path and environment wiring
// without requiring a private registry.
func Test_BicepConfig_CRUD(t *testing.T) {
	template := "testdata/bicepconfig-test.bicep"
	appName := "bicepconfig-test-app"
	appNamespace := "bicepconfig-test-ns"

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
			// Deploy the template with bicepConfig, environment, and application.
			Executor:                               step.NewDeployExecutor(template, "appName="+appName),
			SkipObjectValidation:                   true,
			SkipKubernetesOutputResourceValidation: true,
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "test-bicep-config",
						Type: "radius.core/bicepconfigs",
					},
					{
						Name: "bicepconfig-test-env",
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
