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
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/project-radius/radius/pkg/ucp/resources"
	resources_radius "github.com/project-radius/radius/pkg/ucp/resources/radius"
	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/shared"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
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
	secret, err := getSecretSuffix("/planes/radius/local/resourcegroups/kind-radius/providers/Applications.Core/extenders/"+name, envName, appName)
	require.NoError(t, err)
	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetTerraformRecipeModuleServerURL(), "appName="+appName, "redisCacheName="+redisCacheName),
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
						Name:            name,
						Type:            validation.ExtendersResource,
						App:             appName,
						OutputResources: []validation.OutputResourceResponse{}, // No output resources because Terraform Recipe outputs aren't integreted yet.
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appName: {
						validation.NewK8sServiceForResource(appName, redisCacheName).ValidateLabels(false),
					},
					"radius-system": {
						validation.NewK8sSecretForResourceWithResourceName("tfstate-default-" + secret).ValidateLabels(false),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test shared.RPTest) {
				resourceID := "/planes/radius/local/resourcegroups/kind-radius/providers/Applications.Core/extenders/" + name
				testSecretDeletion(t, ctx, test, appName, envName, resourceID)
			},
		},
	})
	test.Test(t)
}

func Test_TerraformRecipe_Context(t *testing.T) {
	template := "testdata/corerp-resources-terraform-context.bicep"
	name := "corerp-resources-terraform-context"
	appNamespace := "corerp-resources-terraform-context-app"
	secret, err := getSecretSuffix("/planes/radius/local/resourcegroups/kind-radius/providers/Applications.Core/extenders/"+name, name, name)
	require.NoError(t, err)
	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetTerraformRecipeModuleServerURL()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
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
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sSecretForResource(name, name),
					},
					"radius-system": {
						validation.NewK8sSecretForResourceWithResourceName("tfstate-default-" + secret).ValidateLabels(false),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test shared.RPTest) {
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
			},
			SkipResourceDeletion: true,
		},
	})
	test.Test(t)
}

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
				testSecretDeletion(t, ctx, test, appName, envName, resourceID)
			},
		},
	})
	test.Test(t)
}

func testSecretDeletion(t *testing.T, ctx context.Context, test shared.RPTest, appName, envName, resourceID string) {
	secretSuffix, err := getSecretSuffix(resourceID, envName, appName)
	require.NoError(t, err)

	secret, err := test.Options.K8sClient.CoreV1().Secrets(appName).
		Get(ctx, "tfstate-default-"+secretSuffix, metav1.GetOptions{})
	require.Error(t, err)
	require.True(t, apierrors.IsNotFound(err))
	require.Equal(t, secret, &corev1.Secret{})
}

func getSecretSuffix(resourceID, envName, appName string) (string, error) {
	parsedResourceID, err := resources.Parse(resourceID)
	if err != nil {
		return "", err
	}

	prefix := fmt.Sprintf("%s-%s-%s", envName, appName, parsedResourceID.Name())
	maxResourceNameLen := 22
	if len(prefix) >= maxResourceNameLen {
		prefix = prefix[:maxResourceNameLen]
	}

	hasher := sha1.New()
	_, err = hasher.Write([]byte(strings.ToLower(fmt.Sprintf("%s-%s-%s", envName, appName, parsedResourceID.String()))))
	if err != nil {
		return "", err
	}
	hash := hasher.Sum(nil)

	// example: env-app-redis.ec291e26078b7ea8a74abfac82530005a0ecbf15
	return fmt.Sprintf("%s.%x", prefix, hash), nil
}
