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

// Test_DynamicRP_Recipe tests creation of a resource using a user-defined type and its associated recipe.
// The test consists of two main steps:
//
// 1. Resource Type Registration:
//   - Registers a user-defined resource type "Test.Resources/userTypeAlpha"
//   - Verifies the registration by checking if the resource type is listed in the CLI output
//
// 2. Resource Deployment:
//   - Deploys a Bicep template that uses the registered resource type with a default recipe
//   - Validates the creation of required Radius resources (environment and application)
//   - Verifies the creation of Kubernetes objects (pod) in the specified namespace
//   - Confirms the resource's status shows correct binding configuration
func Test_DynamicRP_Recipe(t *testing.T) {
	template := "testdata/usertypealpha-recipe.bicep"
	name := "usertypealpha-recipe-app"
	appNamespace := "default-usertypealpha-recipe"
	resourceTypeName := "Test.Resources/userTypeAlpha"
	filepath := "testdata/usertypealpha.yaml"
	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			// The first step in this test is to create/register a user-defined resource type using the CLI.
			Executor: step.NewFuncExecutor(func(ctx context.Context, t *testing.T, options test.TestOptions) {
				_, err := cli.ResourceProviderCreate(ctx, filepath)
				require.NoError(t, err)
			}),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				output, err := cli.RunCommand(ctx, []string{"resource-type", "show", resourceTypeName, "--output", "json"})
				require.NoError(t, err)
				require.Contains(t, output, resourceTypeName)
			},
		},
		{
			// The next step is to deploy a bicep file using a default recipe for the resource type registered.
			Executor: step.NewDeployExecutor(template, testutil.GetBicepRecipeRegistry(), testutil.GetBicepRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "usertypealpha-recipe-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "usertypealpha").ValidateLabels(false),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				usertypealpha, err := test.Options.ManagementClient.GetResource(ctx, "Test.Resources/userTypeAlpha", "usertypealpha123")
				require.NoError(t, err)
				require.NotNil(t, usertypealpha)
				status := usertypealpha.Properties["status"].(map[string]any)
				binding := status["binding"].(map[string]interface{})
				require.Equal(t, "8080", binding["port"].(string))
			},
		},
	})

	test.Test(t)
}

// This test verifies deployment of shared environment scoped resource using 'existing' keyword.
// It has 2 steps:
// 1. Deploy the environment and postgres resource to the environment namespace.
// 2. Deploy an app that uses the existing postgres resource using the 'existing' keyword.
func Test_Postgres_EnvScoped_ExistingResource(t *testing.T) {
	envTemplate := "testdata/postgres-env-scoped-resource.bicep"
	existingTemplate := "testdata/postgres-existing-and-cntr.bicep"
	name := "dynamicrp-postgres-env"
	appNamespace := "dynamicrp-postgres-existing-app"
	appName := "dynamicrp-postgres-existing"
	resourceTypeName := "Test.Resources/postgres"
	filepath := "testdata/usertypealpha.yaml"
	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)
	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			// The first step in this test is to create/register a user-defined resource type using the CLI.
			Executor: step.NewFuncExecutor(func(ctx context.Context, t *testing.T, options test.TestOptions) {
				_, err := cli.ResourceProviderCreate(ctx, filepath)
				require.NoError(t, err)
			}),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				output, err := cli.RunCommand(ctx, []string{"resource-type", "show", resourceTypeName, "--output", "json"})
				require.NoError(t, err)
				require.Contains(t, output, resourceTypeName)
			},
		},
		{
			Executor: step.NewDeployExecutor(envTemplate, testutil.GetBicepRecipeRegistry(), testutil.GetBicepRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.EnvironmentsResource,
					},
					{
						Name: "existing-postgres",
						Type: "test.resources/postgres",
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					name: {
						validation.NewK8sPodForResource("postgresql", "postgresql").ValidateLabels(false),
					},
				},
			},
		},
		{
			Executor: step.NewDeployExecutor(existingTemplate),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: appName,
						Type: validation.ApplicationsResource,
						App:  appName,
					},
					{
						Name: "postgres-cntr",
						Type: validation.ContainersResource,
						App:  appName,
					},
					{
						Name: "existing-postgres",
						Type: "test.resources/postgres",
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "postgres-cntr").ValidateLabels(false),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				// Verify that the environment namespace is created.
				_, err := ct.Options.K8sClient.CoreV1().Namespaces().Get(context.Background(), name, metav1.GetOptions{})
				require.NoError(t, err)
			},
		},
	})
	test.Test(t)
}

// Test_DynamicRP_ExternalResource tests the deployment of a user-defined resource type with external resources.
// It consists of two main steps:
// 1. Resource Type Registration:
//   - Registers a user-defined resource type "Test.Resources/externalResource"
//
// 2. Resource Deployment:
//   - Creates a Kubernetes ConfigMap containing configuration properties
//   - Deploys a custom resource instance of type Test.Resources/externalResource that reads the ConfigMap data
//   - Launches a container that receives the ConfigMap data via environment variables
//   - Validates the environment variable in the container to ensure it contains the expected value
func Test_DynamicRP_ExternalResource(t *testing.T) {
	template := "testdata/externalresource.bicep"
	appName := "udt-externalresource-app"
	appNamespace := "udt-externalresource-env-udt-externalresource-app"
	expectedEnvValue := `{"app1.sample.properties":"property1=value1\nproperty2=value2","app2.sample.properties":"property3=value3\nproperty4=value4"}`
	resourceTypeName := "Test.Resources/externalResource"
	containerName := "externalresourcecntr"
	filepath := "testdata/usertypealpha.yaml"
	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	test := rp.NewRPTest(t, appName, []rp.TestStep{
		{
			// The first step in this test is to create/register a user-defined resource type using the CLI.
			Executor: step.NewFuncExecutor(func(ctx context.Context, t *testing.T, options test.TestOptions) {
				_, err := cli.ResourceProviderCreate(ctx, filepath)
				require.NoError(t, err)
			}),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				output, err := cli.RunCommand(ctx, []string{"resource-type", "show", resourceTypeName, "--output", "json"})
				require.NoError(t, err)
				require.Contains(t, output, resourceTypeName)
			},
		},
		{
			Executor: step.NewDeployExecutor(template),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "udt-externalresource-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: appName,
						Type: validation.ApplicationsResource,
					},
					{
						Name: containerName,
						Type: validation.ContainersResource,
					},
					{
						Name: "udt-externalresource",
						Type: resourceTypeName,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sConfigMapForResource("udt-config-map").ValidateLabels(false)},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				// Verify environment variable in the container has expected value
				deploy, err := test.Options.K8sClient.AppsV1().Deployments(appNamespace).Get(ctx, containerName, metav1.GetOptions{})
				require.NoError(t, err)

				var targetContainer *corev1.Container
				for i := range deploy.Spec.Template.Spec.Containers {
					container := &deploy.Spec.Template.Spec.Containers[i]
					if container.Name == containerName {
						targetContainer = container
						break
					}
				}
				require.NotNil(t, targetContainer, "Container not found")

				found := false
				for _, env := range targetContainer.Env {
					if env.Name == "UDTCONFIGMAP_DATA" {
						require.Equal(t, expectedEnvValue, env.Value)
						found = true
						break
					}
				}
				require.True(t, found, "Environment variable not found")
			},
		},
	})
	test.Test(t)
}
