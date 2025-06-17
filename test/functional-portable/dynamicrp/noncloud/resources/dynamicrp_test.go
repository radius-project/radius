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
	"fmt"
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
/*
func Test_DynamicRP_Recipe(t *testing.T) {
	template := "testdata/usertypealpha-recipe.bicep"
	appName := "usertypealpha-recipe-app"
	appNamespace := "usertypealpha-recipe-env-usertypealpha-recipe-app"
	containerName := "usertypealphacntr"
	resourceTypeName := "Test.Resources/userTypeAlpha"
	filepath := "testdata/testresourcetypes.yaml"
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
	filepath := "testdata/testresourcetypes.yaml"
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
	filepath := "testdata/testresourcetypes.yaml"
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

// This test verifies the connections from a container to an UDT  injects the environment variables
// from the UDT into the container.
// It has 2 steps:
// 1. Deploy the environment and postgres resource to the environment namespace.
// 2. Deploy an app that uses a container resource  which connects to the postgres resource.
// Verify env variables are injected automatically into the container.
func Test_Container_ConnectionTo_UDT(t *testing.T) {
	existingTemplate := "testdata/container2udt-connection.bicep"
	name := "dynamicrp-cntr2udt"
	appNamespace := "dynamicrp-cntr2udt"
	appName := "dynamicrp-cntr2udt"
	resourceTypeName := "Test.Resources/postgres"
	filepath := "testdata/testresourcetypes.yaml"
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
			Executor: step.NewDeployExecutor(existingTemplate, testutil.GetBicepRecipeRegistry(), testutil.GetBicepRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: appName,
						Type: validation.ApplicationsResource,
						App:  appName,
					},
					{
						Name: "udtcntr",
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
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				deploys, err := test.Options.K8sClient.AppsV1().Deployments(appNamespace).List(ctx, metav1.ListOptions{})
				if deploys == nil {
					t.Fatalf("No deployments found in namespace %s", appNamespace)
				}
				require.NoError(t, err)
				deploy, err := test.Options.K8sClient.AppsV1().Deployments(appNamespace).Get(ctx, "udtcntr", metav1.GetOptions{})
				require.NoError(t, err)

				var targetContainer *corev1.Container
				for i := range deploy.Spec.Template.Spec.Containers {
					container := &deploy.Spec.Template.Spec.Containers[i]
					if container.Name == "udtcntr" {
						targetContainer = container
						break
					}
				}
				require.NotNil(t, targetContainer, "Container not found")

				// Verify environment variable in the container has expected value
				requiredEnvVars := []string{
					"CONNECTION_POSTGRES_HOST",
					"CONNECTION_POSTGRES_PORT",
					"CONNECTION_POSTGRES_DATABASE",
					"CONNECTION_POSTGRES_PASSWORD",
					"CONNECTION_POSTGRES_RECIPE",
				}

				// Verify all required environment variables are present
				foundEnvVars := make(map[string]bool)
				for _, env := range targetContainer.Env {
					foundEnvVars[env.Name] = true
				}

				for _, requiredVar := range requiredEnvVars {
					require.True(t, foundEnvVars[requiredVar],
						"Required environment variable %s not found in container", requiredVar)
				}

				for _, env := range targetContainer.Env {
					require.NotNil(t, env.ValueFrom)
					require.NotNil(t, env.ValueFrom.SecretKeyRef)
					require.Equal(t, env.Name, env.ValueFrom.SecretKeyRef.Key)
				}

			},
		},
	})
	test.Test(t)
}

func Test_UDT_ConnectionTo_UDT(t *testing.T) {
	existingTemplate := "testdata/udt2udt-connection.bicep"
	name := "dynamicrp-udt2udt"
	appNamespace := "udttoudtapp"
	appName := "udttoudtapp"
	childResourceTypeName := "Test.Resources/postgres"
	parentResourceTypeName := "Test.Resources/udtParent"
	filepath := "testdata/testresourcetypes.yaml"
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
				output, err := cli.RunCommand(ctx, []string{"resource-type", "show", childResourceTypeName, "--output", "json"})
				require.NoError(t, err)
				require.Contains(t, output, childResourceTypeName)
				output, err = cli.RunCommand(ctx, []string{"resource-type", "show", parentResourceTypeName, "--output", "json"})
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
						Name: "udtparent",
						Type: "test.resources/udtparent",
						App:  appName,
					},
					{
						Name: "udtchild",
						Type: "test.resources/postgres",
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				deploys, err := test.Options.K8sClient.AppsV1().Deployments(appNamespace).List(ctx, metav1.ListOptions{})
				require.NoError(t, err)
				require.NotEmpty(t, deploys.Items, "No deployments found in namespace %s", appNamespace)

				found := false
				for _, deploy := range deploys.Items {
					t.Logf("Checking deployment: %s", deploy.Name)

					// Check pod template labels
					if deploy.Spec.Template.Labels != nil {
						t.Logf("Pod template labels for deployment '%s':", deploy.Name)
						for key, value := range deploy.Spec.Template.Labels {
							t.Logf("  %s: %s", key, value)
						}

						if labelValue, exists := deploy.Spec.Template.Labels["radapp.io/connected-to-resource"]; exists {
							t.Logf("✓ Found deployment '%s' with label 'radapp.io/connected-to-resource'='%s'", deploy.Name, labelValue)
							found = true

							// Verify the label value is not empty (should contain the host value)
							require.NotEmpty(t, labelValue, "Label value should not be empty")
						}
					}
				}

				require.True(t, found, "No deployments found with label 'radapp.io/connected-to-resource' in pod template labels")

			},
		},
	})
	test.Test(t)
}
*/
func Test_UDT_ConnectionTo_UDTTF(t *testing.T) {
	existingTemplate := "testdata/udt2udt-connection-tf.bicep"
	name := "dynamicrp-udt2udt"
	appNamespace := "udttoudtapp"
	appName := "udttoudtapp"
	childResourceTypeName := "Test.Resources/udtChild"
	parentResourceTypeName := "Test.Resources/udtParent"
	filepath := "testdata/testresourcetypes.yaml"
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
				output, err := cli.RunCommand(ctx, []string{"resource-type", "show", childResourceTypeName, "--output", "json"})
				require.NoError(t, err)
				require.Contains(t, output, childResourceTypeName)
				output, err = cli.RunCommand(ctx, []string{"resource-type", "show", parentResourceTypeName, "--output", "json"})
				require.NoError(t, err)
				require.Contains(t, output, parentResourceTypeName)
			},
		},
		{
			Executor:                               step.NewDeployExecutor(existingTemplate, testutil.GetTerraformRecipeModuleServerURL()), //step.NewDeployExecutor(existingTemplate, testutil.GetBicepRecipeRegistry(), testutil.GetBicepRecipeVersion()),
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
						Name: "udtparent",
						Type: "test.resources/udtparent",
						App:  appName,
					},
					{
						Name: "udtchild",
						Type: "test.resources/udtChild",
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				deploys, err := test.Options.K8sClient.AppsV1().Deployments(appNamespace).List(ctx, metav1.ListOptions{})
				if deploys == nil {
					t.Fatalf("No deployments found in namespace %s", appNamespace)
				}
				require.NoError(t, err)
				//list deployments
				fmt.Println("Deployments in namespace:", appNamespace)
				for _, deploy := range deploys.Items {
					fmt.Println("Deployment Name:", deploy.Name)
				}
				fmt.Println("Total Deployments Found:", len(deploys.Items))
				deploy, err := test.Options.K8sClient.AppsV1().Deployments(appNamespace).Get(ctx, "udtparent", metav1.GetOptions{})
				require.NoError(t, err)

				var targetContainer *corev1.Container
				for i := range deploy.Spec.Template.Spec.Containers {
					container := &deploy.Spec.Template.Spec.Containers[i]
					fmt.Println("Container Name:", container.Name)
					if container.Name == "postgres" {
						targetContainer = container
						break
					}
				}
				require.NotNil(t, targetContainer, "Container not found")

				// Verify environment variable in the container has expected value
				requiredEnvVars := []string{
					"CONNECTED_TO",
				}

				// Verify all required environment variables are present
				foundEnvVars := make(map[string]bool)
				for _, env := range targetContainer.Env {
					fmt.Print("Environment Variable Found: ", env.Name, "\n")
					foundEnvVars[env.Name] = true
				}
				for _, requiredVar := range requiredEnvVars {
					require.True(t, foundEnvVars[requiredVar],
						"Required environment variable %s not found in container", requiredVar)
				}
			},
		},
	})
	test.Test(t)
}
