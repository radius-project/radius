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
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/test/functional"
	"github.com/radius-project/radius/test/functional/shared"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
)

// This file contains tests for Bicep recipes functionality - covering general behaviors that should
// be consistent across all resource types. These tests mostly use the extender resource type and mostly
// avoid cloud resources to avoid unnecessary coupling and reliability issues.
//
// Tests in this file should only use cloud resources if absolutely necessary.
//
// Tests in this file should be kept *roughly* in sync with recipe_terraform_test and any other drivers.

// This tests parameters on the input side and values/secrets on the output side.
func Test_BicepRecipe_ParametersAndOutputs(t *testing.T) {
	template := "testdata/corerp-resources-recipe-bicep.bicep"
	name := "corerp-resources-recipe-bicep-parametersandoutputs"

	// Best way to pass complex parameters is to use JSON.
	parametersFilePath := functional.WriteBicepParameterFile(t, map[string]any{
		// These will be set on the environment as part of the recipe
		"environmentParameters": map[string]any{
			"a": "environment",
			"d": "environment",
		},

		// These will be set on the extender resource
		"resourceParameters": map[string]any{
			"c": 42,
			"d": "resource",
		},
	})

	parameters := []string{
		functional.GetBicepRecipeRegistry(),
		functional.GetBicepRecipeVersion(),
		fmt.Sprintf("basename=%s", name),
		fmt.Sprintf("recipe=%s", "parameters-outputs"),
		"@" + parametersFilePath,
	}

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, parameters...),
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
			K8sObjects: &validation.K8sObjectSet{},
			PostStepVerify: func(ctx context.Context, t *testing.T, test shared.RPTest) {
				resource, err := test.Options.ManagementClient.ShowResource(ctx, "Applications.Core/extenders", name)
				require.NoError(t, err)

				text, err := json.MarshalIndent(resource, "", "  ")
				require.NoError(t, err)
				t.Logf("resource data:\n %s", text)

				require.Equal(t, "environment", resource.Properties["a"])
				require.Equal(t, "default value", resource.Properties["b"])
				require.Equal(t, 42.0, resource.Properties["c"])
				require.Equal(t, "resource", resource.Properties["d"])

				response, err := test.Options.CustomAction.InvokeCustomAction(ctx, *resource.ID, "2022-03-15-privatepreview", "listSecrets")
				require.NoError(t, err)

				t.Logf("secret data:\n %+v", response.Body)

				expected := map[string]any{"e": "so secret"}
				require.Equal(t, expected, response.Body)
			},
		},
	})
	test.Test(t)
}

// This test actually creates a Radius resource using a recipe (yeah, not a real user scenario).
//
// The purpose of this test is to test creation and behavior of **output resources**. This way we
// can test the behavior of the output resource without having to create a real cloud resource.
//
// The reason we test both a Radius resource and a Kubernetes resource is that the deployment
// engine has different behaviors for these cases. For Kubernetes resources (at the time of writing)
// users need to manually include them in the output resources. For a Radius resource (or any UCP/Azure)
// resource type including it in output resources is automatic.
//
// There are four cases we want to test (determined by open-box analysis):
//
// - Creating a UCP resource
// - Creating a Kubernetes resource and manually including it in output resources
// - Creating a resource in a module
// - Referencing an existing resource
//
// Each of these cases requires a distinct behavior from the driver.
func Test_BicepRecipe_ResourceCreation(t *testing.T) {
	templateFmt := "testdata/corerp-resources-recipe-bicep-resourcecreation.%s.bicep"
	name := "corerp-resources-recipe-bicep-resourcecreation"

	parametersStep0 := []string{
		functional.GetBicepRecipeRegistry(),
		functional.GetBicepRecipeVersion(),
		fmt.Sprintf("basename=%s", name),
		fmt.Sprintf("recipe=%s", "resource-creation"),
	}

	parametersStep1 := []string{
		fmt.Sprintf("basename=%s", name),
	}

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(fmt.Sprintf(templateFmt, "step0"), parametersStep0...),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: name + "-existing",
						Type: validation.ExtendersResource,
					},
				},
			},
			SkipResourceDeletion: true,
			K8sObjects:           &validation.K8sObjectSet{},
		},
		{
			Executor: step.NewDeployExecutor(fmt.Sprintf(templateFmt, "step1"), parametersStep1...),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: name + "-existing",
						Type: validation.ExtendersResource,
					},
					{
						Name: name, // Using a recipe
						Type: validation.ExtendersResource,
					},
					{
						Name: name + "-created", // Created inside the recipe
						Type: validation.ExtendersResource,
					},
					{
						Name: name + "-module", // Created inside the recipe using a module
						Type: validation.ExtendersResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{},
			// Trying to delete the resources can cause multiple concurrent delete requests.
			// This currently fails.
			SkipResourceDeletion: true,
			PostStepVerify: func(ctx context.Context, t *testing.T, test shared.RPTest) {
				resource, err := test.Options.ManagementClient.ShowResource(ctx, "Applications.Core/extenders", name)
				require.NoError(t, err)

				text, err := json.MarshalIndent(resource, "", "  ")
				require.NoError(t, err)
				t.Logf("resource data:\n %s", text)

				// Let's verify the output resources.
				parsed, err := resources.ParseResource(*resource.ID)
				require.NoError(t, err)

				scope := strings.ReplaceAll(parsed.RootScope(), "resourcegroups", "resourceGroups")
				expected := []any{
					map[string]any{
						"id":            "/planes/kubernetes/local/namespaces/" + name + "-app/providers/core/Secret/" + name,
						"radiusManaged": true,
					},
					map[string]any{
						"id":            scope + "/providers/Applications.Core/extenders/" + name + "-created",
						"radiusManaged": true,
					}, map[string]interface{}{
						"id":            scope + "/providers/Applications.Core/extenders/" + name + "-module",
						"radiusManaged": true,
					},
				}
				actual := resource.Properties["status"].(map[string]any)["outputResources"].([]any)
				require.Equal(t, expected, actual)
			},
		},
	})
	test.Test(t)
}

func Test_BicepRecipe_ParameterNotDefined(t *testing.T) {
	template := "testdata/corerp-resources-recipe-bicep.bicep"
	name := "corerp-resources-recipe-bicep-parameternotdefined"

	// Best way to pass complex parameters is to use JSON.
	parametersFilePath := functional.WriteBicepParameterFile(t, map[string]any{
		// These will be set on the environment as part of the recipe
		"environmentParameters": map[string]any{
			"a": "environment",
		},

		// These will be set on the extender resource
		"resourceParameters": map[string]any{
			"b": "resource",
		},
	})

	parameters := []string{
		functional.GetBicepRecipeRegistry(),
		functional.GetBicepRecipeVersion(),
		fmt.Sprintf("basename=%s", name),
		fmt.Sprintf("recipe=%s", "empty-recipe"),
		"@" + parametersFilePath,
	}

	validate := step.ValidateSingleDetail("DeploymentFailed", step.DeploymentErrorDetail{
		Code: "ResourceDeploymentFailure",
		Details: []step.DeploymentErrorDetail{
			{
				Code: recipes.RecipeDeploymentFailed,
				// NOTE: There is a bug in our error handling for deployements. We return the JSON text of the deployment error inside the message
				// of our error. This is wrong.
				//
				// See: https://github.com/radius-project/radius/issues/6045

				MessageContains: "Deployment template validation failed: 'The template parameters 'a, b' in the parameters file are not valid",
			},
		},
	})

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployErrorExecutor(template, validate, parameters...),
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
			K8sObjects: &validation.K8sObjectSet{},
		},
	})
	test.Test(t)
}

func Test_BicepRecipe_WrongOutput(t *testing.T) {
	template := "testdata/corerp-resources-recipe-bicep.bicep"
	name := "corerp-resources-recipe-bicep-wrongoutput"
	parameters := []string{
		functional.GetBicepRecipeRegistry(),
		functional.GetBicepRecipeVersion(),
		fmt.Sprintf("basename=%s", name),
		fmt.Sprintf("recipe=%s", "wrong-output"),
	}

	validate := step.ValidateSingleDetail("DeploymentFailed", step.DeploymentErrorDetail{
		Code: "ResourceDeploymentFailure",
		Details: []step.DeploymentErrorDetail{
			{
				Code:            recipes.InvalidRecipeOutputs,
				MessageContains: "failed to read the recipe output \"result\": json: unknown field \"error\"",
			},
		},
	})

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployErrorExecutor(template, validate, parameters...),
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
			K8sObjects: &validation.K8sObjectSet{},
		},
	})
	test.Test(t)
}

func Test_BicepRecipe_LanguageFailure(t *testing.T) {
	template := "testdata/corerp-resources-recipe-bicep.bicep"
	name := "corerp-resources-recipe-bicep-langugagefailure"
	parameters := []string{
		functional.GetBicepRecipeRegistry(),
		functional.GetBicepRecipeVersion(),
		fmt.Sprintf("basename=%s", name),
		fmt.Sprintf("recipe=%s", "language-failure"),
	}

	validate := step.ValidateSingleDetail("DeploymentFailed", step.DeploymentErrorDetail{
		Code: "ResourceDeploymentFailure",
		Details: []step.DeploymentErrorDetail{
			{
				Code: recipes.RecipeDeploymentFailed,
				// NOTE: There is a bug in our error handling for deployements. We return the JSON text of the deployment error inside the message
				// of our error. This is wrong.
				//
				// See: https://github.com/radius-project/radius/issues/6046

				MessageContains: "Unable to process template language expressions for resource",
			},
		},
	})

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployErrorExecutor(template, validate, parameters...),
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
			K8sObjects: &validation.K8sObjectSet{},
		},
	})
	test.Test(t)
}

func Test_BicepRecipe_ResourceCreationFailure(t *testing.T) {
	template := "testdata/corerp-resources-recipe-bicep.bicep"
	name := "corerp-resources-recipe-bicep-resourcecreationfailure"
	parameters := []string{
		functional.GetBicepRecipeRegistry(),
		functional.GetBicepRecipeVersion(),
		fmt.Sprintf("basename=%s", name),
		fmt.Sprintf("recipe=%s", "resource-creation-failure"),
	}

	validate := step.ValidateSingleDetail("DeploymentFailed", step.DeploymentErrorDetail{
		Code: "ResourceDeploymentFailure",
		Details: []step.DeploymentErrorDetail{
			{
				Code: recipes.RecipeDeploymentFailed,
				// NOTE: There is a bug in our error handling for deployements. We return the JSON text of the deployment error inside the message
				// of our error. This is wrong.
				//
				// See: https://github.com/radius-project/radius/issues/6047

				MessageContains: "'not an id, just deal with it' is not a valid resource id",
			},
		},
	})

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployErrorExecutor(template, validate, parameters...),
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
			K8sObjects: &validation.K8sObjectSet{},
		},
	})
	test.Test(t)
}
