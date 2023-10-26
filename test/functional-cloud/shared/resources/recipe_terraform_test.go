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
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/backends"
	"github.com/radius-project/radius/test/functional"
	"github.com/radius-project/radius/test/functional/shared"
	"github.com/radius-project/radius/test/step"
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

// Test_TerraformRecipe_AzureStorage creates an Extender resource consuming a Terraform recipe that deploys an Azure blob storage instance.
func Test_TerraformRecipe_AzureStorage(t *testing.T) {
	template := "testdata/corerp-resources-terraform-azurestorage.bicep"
	name := "corerp-resources-terraform-azstorage"
	appName := "corerp-resources-terraform-azstorage-app"
	envName := "corerp-resources-terraform-azstorage-env"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetTerraformRecipeModuleServerURL(), "appName="+appName),
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
			PostStepVerify: func(ctx context.Context, t *testing.T, test shared.RPTest) {
				resourceID := "/planes/radius/local/resourcegroups/kind-radius/providers/Applications.Core/extenders/" + name
				secretSuffix, err := getSecretSuffix(resourceID, envName, appName)
				require.NoError(t, err)

				secret, err := test.Options.K8sClient.CoreV1().Secrets(secretNamespace).
					Get(ctx, secretPrefix+secretSuffix, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, secretNamespace, secret.Namespace)
				require.Equal(t, secretPrefix+secretSuffix, secret.Name)
			},
		},
	})

	test.PostDeleteVerify = func(ctx context.Context, t *testing.T, test shared.RPTest) {
		resourceID := "/planes/radius/local/resourcegroups/kind-radius/providers/Applications.Core/extenders/" + name
		testSecretDeletion(t, ctx, test, appName, envName, resourceID)
	}

	test.Test(t)
}

func testSecretDeletion(t *testing.T, ctx context.Context, test shared.RPTest, appName, envName, resourceID string) {
	secretSuffix, err := getSecretSuffix(resourceID, envName, appName)
	require.NoError(t, err)

	secret, err := test.Options.K8sClient.CoreV1().Secrets(secretNamespace).
		Get(ctx, secretPrefix+secretSuffix, metav1.GetOptions{})
	require.Error(t, err)
	require.True(t, apierrors.IsNotFound(err))
	require.Equal(t, secret, &corev1.Secret{})
}

func getSecretSuffix(resourceID, envName, appName string) (string, error) {
	envID := "/planes/radius/local/resourcegroups/kind-radius/providers/Applications.Core/environments/" + envName
	appID := "/planes/radius/local/resourcegroups/kind-radius/providers/Applications.Core/applications/" + appName

	resourceRecipe := recipes.ResourceMetadata{
		EnvironmentID: envID,
		ApplicationID: appID,
		ResourceID:    resourceID,
		Parameters:    nil,
	}

	backend := backends.NewKubernetesBackend(nil)
	secretMap, err := backend.BuildBackend(&resourceRecipe)
	if err != nil {
		return "", err
	}
	kubernetes := secretMap["kubernetes"].(map[string]any)

	return kubernetes["secret_suffix"].(string), nil
}
