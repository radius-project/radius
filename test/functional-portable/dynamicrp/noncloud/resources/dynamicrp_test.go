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
