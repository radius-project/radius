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

// Test_DynamicRP_Recipe tests creation of a resource using a user-defined type and its associated recipe.
// The test consists of two main steps:
//
// 1. Resource Type Registration:
//   - Registers a user-defined resource type "Test.Resources/userTypeAlpha"
//   - Verifies the registration by checking if the resource type is listed in the CLI output
//
// 2. Resource Deployment:
//   - Deploys a Bicep template that uses the registered resource type with a default recipe and a recipe parameter: port
//   - Validates the creation of required resources (app, container, environment
//     and the user-defined resource instance) in the Kubernetes cluster
//   - Validates that the container can access port information from the user-defined resource via environment variables
func Test_DynamicRP_Recipe(t *testing.T) {
	template := "testdata/usertypealpha-recipe.bicep"
	appName := "usertypealpha-recipe-app"
	appNamespace := "usertypealpha-recipe-env-usertypealpha-recipe-app"
	containerName := "usertypealphacntr"
	resourceTypeName := "Test.Resources/userTypeAlpha"
	resourceTypeParam := strings.Split(resourceTypeName, "/")[1]
	filepath := "testdata/testresourcetypes.yaml"
	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	test := rp.NewRPTest(t, appName, []rp.TestStep{
		{
			// The first step in this test is to create/register a user-defined resource type using the CLI.
			Executor: step.NewFuncExecutor(func(ctx context.Context, t *testing.T, options test.TestOptions) {
				_, err := cli.ResourceTypeCreate(ctx, resourceTypeParam, filepath)
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
						Name: appName,
						Type: validation.ApplicationsResource,
					},
					{
						Name: containerName,
						Type: validation.ContainersResource,
					},
					{
						Name: "usertypealphainstance",
						Type: resourceTypeName,
					},
					{
						Name: "usertypealphalatest",
						Type: resourceTypeName,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(appName, "usertypealpha").ValidateLabels(false),
					},
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
					if env.Name == "USERTYPEALPHA_PORT" {
						require.Equal(t, "8080", env.Value)
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
	resourceTypeParam := strings.Split(resourceTypeName, "/")[1]
	filepath := "testdata/testresourcetypes.yaml"
	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)
	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			// The first step in this test is to create/register a user-defined resource type using the CLI.
			Executor: step.NewFuncExecutor(func(ctx context.Context, t *testing.T, options test.TestOptions) {
				_, err := cli.ResourceTypeCreate(ctx, resourceTypeParam, filepath)
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
//   - Launches a container that receives the ConfigMap data via environment variables by defining a connection to the external resource
//   - Validates the environment variable in the container and ensures it contains the expected value
func Test_DynamicRP_ExternalResource(t *testing.T) {
	template := "testdata/externalresource.bicep"
	appName := "udt-externalresource-app"
	appNamespace := "udt-externalresource-env-udt-externalresource-app"
	expectedEnvName := "CONNECTION_EXTERNALRESOURCE_CONFIGMAP"
	expectedEnvValue := `{"app1.sample.properties":"property1=value1\nproperty2=value2","app2.sample.properties":"property3=value3\nproperty4=value4"}`
	resourceTypeName := "Test.Resources/externalResource"
	resourceTypeParam := strings.Split(resourceTypeName, "/")[1]
	containerName := "externalresourcecntr"
	filepath := "testdata/testresourcetypes.yaml"
	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	test := rp.NewRPTest(t, appName, []rp.TestStep{
		{
			// The first step in this test is to create/register a user-defined resource type using the CLI.
			Executor: step.NewFuncExecutor(func(ctx context.Context, t *testing.T, options test.TestOptions) {
				_, err := cli.ResourceTypeCreate(ctx, resourceTypeParam, filepath)
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
				var secretName, secretKey string
				for _, env := range targetContainer.Env {
					if env.Name == expectedEnvName {
						require.NotNil(t, env.ValueFrom)
						require.NotNil(t, env.ValueFrom.SecretKeyRef)
						require.Equal(t, env.Name, env.ValueFrom.SecretKeyRef.Key)
						secretName = env.ValueFrom.SecretKeyRef.Name
						secretKey = env.ValueFrom.SecretKeyRef.Key
						found = true
						break
					}
				}
				require.True(t, found, "Environment variable %q not found", expectedEnvName)

				// Verify the secret contains the expected value
				secret, err := test.Options.K8sClient.CoreV1().Secrets(appNamespace).Get(ctx, secretName, metav1.GetOptions{})
				require.NoError(t, err)
				require.NotNil(t, secret.Data)

				secretValue, exists := secret.Data[secretKey]
				require.True(t, exists, "Secret key %s not found in secret %s", secretKey, secretName)
				require.Equal(t, expectedEnvValue, string(secretValue), "Secret value does not match expected value")
			},
		},
	})
	test.Test(t)
}

// Test_UDT_ConnectionTo_UDT tests the deployment of a user-defined resource type with connections to another UDT.
// It consists of two main steps:
// 1. Resource Type Registration:
//   - Registers a user-defined resource type "Test.Resources/externalResource" and "Test.Resources/userTypeAlpha" using the CLI.
//
// 2. Resource Deployment:
//   - Deploys a Bicep template that creates a UDT instance and connects it to another UDT instance.
//   - Validates the creation of required resources in the Kubernetes cluster.
//   - Validates that the UDT parent resource contains the expected environment variable injected and populated with the correct value
//     from the connected UDT child resource.
func Test_UDT_ConnectionTo_UDT(t *testing.T) {
	existingTemplate := "testdata/udt2udt-connection.bicep"
	name := "dynamicrp-udt2udt"
	appNamespace := "udttoudtapp"
	appName := "udttoudtapp"
	childResourceTypeName := "Test.Resources/externalResource"
	childResourceTypeParam := strings.Split(childResourceTypeName, "/")[1]
	parentResourceTypeName := "Test.Resources/userTypeAlpha"
	parentResourceTypeParam := strings.Split(parentResourceTypeName, "/")[1]
	filepath := "testdata/testresourcetypes.yaml"
	expectedEnvName := "CONN_INJECTED"
	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)
	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			// The first step in this test is to create/register required user-defined resource types using the CLI.
			Executor: step.NewFuncExecutor(func(ctx context.Context, t *testing.T, options test.TestOptions) {
				_, err := cli.ResourceTypeCreate(ctx, childResourceTypeParam, filepath)
				require.NoError(t, err)
			}),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				output, err := cli.RunCommand(ctx, []string{"resource-type", "show", childResourceTypeName, "--output", "json"})
				require.NoError(t, err)
				require.Contains(t, output, childResourceTypeName)
			},
		},
		{
			// The first step in this test is to create/register required user-defined resource types using the CLI.
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
			Executor:                               step.NewDeployExecutor(existingTemplate, testutil.GetBicepRecipeRegistry(), testutil.GetBicepRecipeVersion()),
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
			SkipKubernetesOutputResourceValidation: true,
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: appName,
						Type: validation.ApplicationsResource,
						App:  appName,
					},
					{
						Name: "udttoudtparent",
						Type: "test.resources/usertypealpha",
						App:  appName,
					},
					{
						Name: "udttoudtchild",
						Type: "test.resources/externalresource",
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				deploys, err := test.Options.K8sClient.AppsV1().Deployments(appNamespace).List(ctx, metav1.ListOptions{})
				require.NoError(t, err)
				require.NotEmpty(t, deploys.Items, "No deployments found in namespace %s", appNamespace)

				// Expected configMap data from the external resource
				expectedConfigMapData := `{"app1.sample.properties":"property1=value1\nproperty2=value2","app2.sample.properties":"property3=value3\nproperty4=value4"}`

				found := false
				for _, deploy := range deploys.Items {
					t.Logf("Checking deployment: %s", deploy.Name)

					if len(deploy.Spec.Template.Spec.Containers) > 0 {
						container := deploy.Spec.Template.Spec.Containers[0]
						t.Logf("Environment variables for container '%s':", container.Name)

						for _, env := range container.Env {
							t.Logf("  %s: %s", env.Name, env.Value)
							if env.Name == expectedEnvName {
								t.Logf("✓ Found deployment %q with env var %q = %q", deploy.Name, expectedEnvName, env.Value)
								found = true
								require.NotEmpty(t, env.Value, "Environment variable value should not be empty")
								require.Equal(t, expectedConfigMapData, env.Value, "Environment variable should contain configMap data from connected externalResource")
								break
							}
						}
					}
				}

				require.True(t, found, "No deployments found with environment variable %q", expectedEnvName)

			},
		},
	})
	test.Test(t)
}

// Test_UDT_ConnectionTo_UDTTF tests the deployment of a user-defined resource type with connections to another UDT using terraform recipe.
// It consists of two main steps:
// 1. Resource Type Registration:
//   - Registers user-defined resource types "test.resources/usertypealpha" (parent) and "test.resources/externalresource" (child) using the CLI.
//
// 2. Resource Deployment:
//   - Deploys a Bicep template that creates a UDT instance and connects it to an external resource UDT instance using terraform recipe.
//   - Validates the creation of required resources in the Kubernetes cluster.
//   - Validates that the UDT parent resource contains the expected environment variable injected and populated with the correct value
//     from the connected UDT child resource.
func Test_UDT_ConnectionTo_UDTTF(t *testing.T) {
	existingTemplate := "testdata/udt2udt-connection-tf.bicep"
	name := "dynamicrp-udt2udt-tf"
	appNamespace := "udttoudtapp"
	appName := "udttoudtapp"
	expectedEnvName := "CONN_INJECTED"
	childResourceTypeName := "Test.Resources/externalResource"
	childResourceTypeParam := strings.Split(childResourceTypeName, "/")[1]
	parentResourceTypeName := "Test.Resources/userTypeAlpha"
	parentResourceTypeParam := strings.Split(parentResourceTypeName, "/")[1]
	filepath := "testdata/testresourcetypes.yaml"
	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)
	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			// The first step in this test is to create/register a user-defined resource type using the CLI.
			Executor: step.NewFuncExecutor(func(ctx context.Context, t *testing.T, options test.TestOptions) {
				_, err := cli.ResourceTypeCreate(ctx, childResourceTypeParam, filepath)
				require.NoError(t, err)
			}),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				output, err := cli.RunCommand(ctx, []string{"resource-type", "show", childResourceTypeName, "--output", "json"})
				require.NoError(t, err)
				require.Contains(t, output, childResourceTypeName)
			},
		},
		{
			// The first step in this test is to create/register a user-defined resource type using the CLI.
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
			Executor:                               step.NewDeployExecutor(existingTemplate, testutil.GetTerraformRecipeModuleServerURL()),
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
			SkipKubernetesOutputResourceValidation: true,
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: appName,
						Type: validation.ApplicationsResource,
						App:  appName,
					},
					{
						Name: "udttoudtparent",
						Type: "test.resources/usertypealpha",
						App:  appName,
					},
					{
						Name: "udttoudtchild",
						Type: "test.resources/externalresource",
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				deploys, err := test.Options.K8sClient.AppsV1().Deployments(appNamespace).List(ctx, metav1.ListOptions{})
				require.NoError(t, err)
				require.NotEmpty(t, deploys.Items, "No deployments found in namespace %s", appNamespace)

				expectedConfigMapData := `{"app1.sample.properties":"property1=value1\nproperty2=value2","app2.sample.properties":"property3=value3\nproperty4=value4"}`

				found := false
				for _, deploy := range deploys.Items {
					t.Logf("Checking deployment: %s", deploy.Name)

					if len(deploy.Spec.Template.Spec.Containers) > 0 {
						container := deploy.Spec.Template.Spec.Containers[0]
						t.Logf("Environment variables for container '%s':", container.Name)

						for _, env := range container.Env {
							t.Logf("  %s: %s", env.Name, env.Value)
							if env.Name == expectedEnvName {
								t.Logf("✓ Found deployment %q with env var %q=%q", deploy.Name, expectedEnvName, env.Value)
								found = true
								require.NotEmpty(t, env.Value, "Environment variable value should not be empty")
								require.Equal(t, expectedConfigMapData, env.Value, "Environment variable should contain configMap data from connected externalResource")
								break
							}
						}
					}
				}

				require.True(t, found, "No deployments found with environment variable %q", expectedEnvName)

			},
		},
	})
	test.Test(t)
}

// Test_DynamicRP_SchemaValidation tests that schema validation properly rejects invalid resources.
// It consists of two main steps:
// 1. Resource Type Registration:
//   - Registers a user-defined resource type "Test.Resources/testResourceSchema" with schema validation
//
// 2. Resource Deployment Failure:
//   - Attempts to deploy a Bicep template with invalid schema (incorrect value, extra properties)
//   - Validates that the deployment fails with appropriate schema validation errors
func Test_DynamicRP_SchemaValidation(t *testing.T) {
	template := "testdata/testResourceSchema-invalid.bicep"
	appName := "udt-schemavalidation-app"
	resourceTypeName := "Test.Resources/testResourceSchema"
	filepath := "testdata/testresourcetypes.yaml"
	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	validate := step.ValidateSingleDetail("DeploymentFailed", step.DeploymentErrorDetail{
		Code: "ResourceDeploymentFailure",
		Details: []step.DeploymentErrorDetail{
			{
				Code:            "InvalidRequestContent",
				MessageContains: "Schema validation failed",
			},
		},
	})

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
			// The next step is to deploy a bicep file with invalid schema - this should fail
			Executor:                               step.NewDeployErrorExecutor(template, validate),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
		},
	})

	test.Test(t)
}

// Test_DynamicRP_TypeAnyValidation_Valid tests that type: any is allowed in additionalProperties under platformOptions.
// It validates that a property named "additionalProperties" with type: any works correctly under platformOptions.
func Test_DynamicRP_TypeAnyValidation_Valid(t *testing.T) {
	template := "testdata/testResourceSchema-validAny.bicep"
	appName := "udt-platformoptions-app"
	appNamespace := "udt-platformoptions-app"
	resourceTypeName := "Test.Resources/testValidPlatformOptionsSchema"
	filepath := "testdata/testresourcetypes.yaml"
	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	test := rp.NewRPTest(t, appName, []rp.TestStep{
		{
			// Register the test resource type
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
			// Deploy bicep with valid type: any usage under platformOptions - should succeed
			Executor: step.NewDeployExecutor(template),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "udt-platformoptions-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: appName,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "udt-valid-platformoptions",
						Type: resourceTypeName,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {},
				},
			},
		},
	})

	test.Test(t)
}
