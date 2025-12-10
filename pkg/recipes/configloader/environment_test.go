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

package configloader

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	model "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	modelv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

const (
	kind            = "kubernetes"
	envNamespace    = "default"
	appNamespace    = "app-default"
	envResourceId   = "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0"
	appResourceId   = "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/app0"
	azureScope      = "/subscriptions/test-sub/resourceGroups/testRG"
	awsScope        = "/planes/aws/aws/accounts/000/regions/cool-region"
	mongoResourceID = "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo-database-0"
	redisID         = "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Datastores/redisCaches/redis-0"

	recipeName      = "cosmosDB"
	terraformRecipe = "terraform-cosmosDB"
)

func TestGetConfiguration(t *testing.T) {
	configTests := []struct {
		name           string
		envResource    *model.EnvironmentResource
		appResource    *model.ApplicationResource
		expectedConfig *recipes.Configuration
		errString      string
	}{
		{
			name: "azure provider with env resource",
			envResource: &model.EnvironmentResource{
				Properties: &model.EnvironmentProperties{
					Compute: &model.KubernetesCompute{
						Kind:       to.Ptr(kind),
						Namespace:  to.Ptr(envNamespace),
						ResourceID: to.Ptr(envResourceId),
					},
					Providers: &model.Providers{
						Azure: &model.ProvidersAzure{
							Scope: to.Ptr(azureScope),
						},
					},
					RecipeConfig: &model.RecipeConfigProperties{
						Terraform: &model.TerraformConfigProperties{
							Authentication: &model.AuthConfig{
								Git: &model.GitAuthConfig{
									Pat: map[string]*model.SecretConfig{
										"dev.azure.com": {
											Secret: to.Ptr("/planes/radius/local/resourceGroups/testGroup/providers/Applications.Core/secretStores/secret"),
										},
									},
								},
							},
						},
					},
				},
			},
			appResource: nil,
			expectedConfig: &recipes.Configuration{
				Runtime: recipes.RuntimeConfiguration{
					Kubernetes: &recipes.KubernetesRuntime{
						Namespace:            envNamespace,
						EnvironmentNamespace: envNamespace,
					},
				},
				Providers: createAzureProvider(),
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Authentication: datamodel.AuthConfig{
							Git: datamodel.GitAuthConfig{
								PAT: map[string]datamodel.SecretConfig{
									"dev.azure.com": {
										Secret: "/planes/radius/local/resourceGroups/testGroup/providers/Applications.Core/secretStores/secret",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "aws provider with env resource",
			envResource: &model.EnvironmentResource{
				Properties: &model.EnvironmentProperties{
					Compute: &model.KubernetesCompute{
						Kind:       to.Ptr(kind),
						Namespace:  to.Ptr(envNamespace),
						ResourceID: to.Ptr(envResourceId),
					},
					RecipeConfig: &model.RecipeConfigProperties{
						Terraform: &model.TerraformConfigProperties{
							Authentication: &model.AuthConfig{
								Git: &model.GitAuthConfig{
									Pat: map[string]*model.SecretConfig{},
								},
							},
						},
					},
					Providers: &model.Providers{
						Aws: &model.ProvidersAws{
							Scope: to.Ptr(awsScope),
						},
					},
				},
			},
			appResource: nil,
			expectedConfig: &recipes.Configuration{
				Runtime: recipes.RuntimeConfiguration{
					Kubernetes: &recipes.KubernetesRuntime{
						Namespace:            envNamespace,
						EnvironmentNamespace: envNamespace,
					},
				},
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Authentication: datamodel.AuthConfig{
							Git: datamodel.GitAuthConfig{
								PAT: map[string]datamodel.SecretConfig{},
							},
						},
					},
				},
				Providers: createAWSProvider(),
			},
		},
		{
			name: "aws provider with env and app resource",
			envResource: &model.EnvironmentResource{
				Properties: &model.EnvironmentProperties{
					Compute: &model.KubernetesCompute{
						Kind:       to.Ptr(kind),
						Namespace:  to.Ptr(envNamespace),
						ResourceID: to.Ptr(envResourceId),
					},
					Providers: &model.Providers{
						Aws: &model.ProvidersAws{
							Scope: to.Ptr(awsScope),
						},
					},
				},
			},
			appResource: &model.ApplicationResource{
				Properties: &model.ApplicationProperties{
					Status: &model.ResourceStatus{
						Compute: &model.KubernetesCompute{
							Kind:       to.Ptr(kind),
							Namespace:  to.Ptr(appNamespace),
							ResourceID: to.Ptr(appResourceId),
						},
					},
				},
			},
			expectedConfig: &recipes.Configuration{
				Runtime: recipes.RuntimeConfiguration{
					Kubernetes: &recipes.KubernetesRuntime{
						Namespace:            "app-default",
						EnvironmentNamespace: envNamespace,
					},
				},
				Providers: createAWSProvider(),
			},
		},
		{
			// Add test here for the simulated flag
			name: "simulated env",
			envResource: &model.EnvironmentResource{
				Properties: &model.EnvironmentProperties{
					Compute: &model.KubernetesCompute{
						Kind:       to.Ptr(kind),
						Namespace:  to.Ptr(envNamespace),
						ResourceID: to.Ptr(envResourceId),
					},
					Simulated: to.Ptr(true),
				},
			},
			appResource: nil,
			expectedConfig: &recipes.Configuration{
				Runtime: recipes.RuntimeConfiguration{
					Kubernetes: &recipes.KubernetesRuntime{
						Namespace:            "default",
						EnvironmentNamespace: envNamespace,
					},
				},
				Simulated: true,
			},
		},
		{
			name: "invalid app resource",
			envResource: &model.EnvironmentResource{
				Properties: &model.EnvironmentProperties{
					Compute: &model.KubernetesCompute{
						Kind:       to.Ptr(kind),
						Namespace:  to.Ptr(envNamespace),
						ResourceID: to.Ptr(envResourceId),
					},
				},
			},
			appResource: &model.ApplicationResource{
				Properties: &model.ApplicationProperties{
					Status: &model.ResourceStatus{
						Compute: &model.EnvironmentCompute{},
					},
				},
			},
			errString: "invalid model conversion",
		},
		{
			name: "invalid env resource",
			envResource: &model.EnvironmentResource{
				Properties: &model.EnvironmentProperties{
					Compute: &model.EnvironmentCompute{
						Kind:       to.Ptr(kind),
						ResourceID: to.Ptr(envResourceId),
					},
					Providers: &model.Providers{
						Azure: &model.ProvidersAzure{
							Scope: to.Ptr(azureScope),
						},
					},
				},
			},
			errString: ErrUnsupportedComputeKind.Error(),
		},
	}

	for _, tc := range configTests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := getConfiguration(tc.envResource, tc.appResource)
			if tc.errString != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errString)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedConfig, result)
			}
		})
	}
}

func createAzureProvider() datamodel.Providers {
	return datamodel.Providers{
		Azure: datamodel.ProvidersAzure{
			Scope: azureScope,
		}}
}

func createAWSProvider() datamodel.Providers {
	return datamodel.Providers{
		AWS: datamodel.ProvidersAWS{
			Scope: awsScope,
		}}
}

func TestGetRecipeDefinition(t *testing.T) {
	envResource := model.EnvironmentResource{
		Properties: &model.EnvironmentProperties{
			Compute: &model.KubernetesCompute{
				Kind:       to.Ptr(kind),
				Namespace:  to.Ptr(envNamespace),
				ResourceID: to.Ptr(envResourceId),
			},
			Providers: &model.Providers{
				Azure: &model.ProvidersAzure{
					Scope: to.Ptr(azureScope),
				},
			},
			Recipes: map[string]map[string]model.RecipePropertiesClassification{
				"Applications.Datastores/mongoDatabases": {
					recipeName: &model.BicepRecipeProperties{
						TemplateKind: to.Ptr(recipes.TemplateKindBicep),
						TemplatePath: to.Ptr("ghcr.io/radius-project/dev/recipes/mongodatabases/azure:1.0"),
						Parameters: map[string]any{
							"foo": "bar",
						},
					},
					"mongo": &model.BicepRecipeProperties{
						TemplateKind: to.Ptr(recipes.TemplateKindBicep),
						TemplatePath: to.Ptr("localhost:8000/recipes/mongodatabases:1.0"),
						PlainHTTP:    to.Ptr(true),
					},
					terraformRecipe: &model.TerraformRecipeProperties{
						TemplateKind:    to.Ptr(recipes.TemplateKindTerraform),
						TemplatePath:    to.Ptr("Azure/cosmosdb/azurerm"),
						TemplateVersion: to.Ptr("1.1.0"),
					},
				},
			},
		},
	}
	recipeMetadata := recipes.ResourceMetadata{
		Name:          recipeName,
		EnvironmentID: envResourceId,
		ResourceID:    mongoResourceID,
	}

	t.Run("invalid resource id", func(t *testing.T) {
		metadata := recipeMetadata
		metadata.ResourceID = "invalid-id"
		_, err := getRecipeDefinition(&envResource, &metadata)
		require.Error(t, err)
		require.Contains(t, err.Error(), "'invalid-id' is not a valid resource id")
	})

	t.Run("recipe not found for the resource type", func(t *testing.T) {
		metadata := recipeMetadata
		metadata.ResourceID = redisID
		_, err := getRecipeDefinition(&envResource, &metadata)
		require.Error(t, err)
		require.Contains(t, err.Error(), "could not find recipe")
	})

	t.Run("success-bicep", func(t *testing.T) {
		expected := recipes.EnvironmentDefinition{
			Name:         recipeName,
			Driver:       recipes.TemplateKindBicep,
			ResourceType: "Applications.Datastores/mongoDatabases",
			TemplatePath: "ghcr.io/radius-project/dev/recipes/mongodatabases/azure:1.0",
			Parameters: map[string]any{
				"foo": "bar",
			},
		}
		recipeDef, err := getRecipeDefinition(&envResource, &recipeMetadata)
		require.NoError(t, err)
		require.Equal(t, recipeDef, &expected)
	})

	t.Run("success-bicep-insecure-registry", func(t *testing.T) {
		metadata := recipes.ResourceMetadata{
			Name:          "mongo",
			EnvironmentID: envResourceId,
			ResourceID:    mongoResourceID,
		}
		expected := recipes.EnvironmentDefinition{
			Name:         "mongo",
			Driver:       recipes.TemplateKindBicep,
			ResourceType: "Applications.Datastores/mongoDatabases",
			TemplatePath: "localhost:8000/recipes/mongodatabases:1.0",
			PlainHTTP:    true,
		}
		recipeDef, err := getRecipeDefinition(&envResource, &metadata)
		require.NoError(t, err)
		require.Equal(t, recipeDef, &expected)
	})
	t.Run("success-terraform", func(t *testing.T) {
		recipeMetadata.Name = terraformRecipe
		expected := recipes.EnvironmentDefinition{
			Name:            terraformRecipe,
			Driver:          recipes.TemplateKindTerraform,
			ResourceType:    "Applications.Datastores/mongoDatabases",
			TemplatePath:    "Azure/cosmosdb/azurerm",
			TemplateVersion: "1.1.0",
		}
		recipeDef, err := getRecipeDefinition(&envResource, &recipeMetadata)
		require.NoError(t, err)
		require.Equal(t, recipeDef, &expected)
	})
	t.Run("no recipes registered to the environment", func(t *testing.T) {
		envResourceNilRecipe := envResource
		envResourceNilRecipe.Properties.Recipes = nil
		_, err := getRecipeDefinition(&envResourceNilRecipe, &recipeMetadata)
		require.Error(t, err)
		require.Contains(t, err.Error(), "could not find recipe")
	})
}

func TestGetConfigurationV20250801(t *testing.T) {
	configTests := []struct {
		name           string
		envResource    *modelv20250801.EnvironmentResource
		appResource    *model.ApplicationResource
		expectedConfig *recipes.Configuration
		errString      string
	}{
		{
			name: "azure provider with env resource v20250801",
			envResource: &modelv20250801.EnvironmentResource{
				Properties: &modelv20250801.EnvironmentProperties{
					Providers: &modelv20250801.Providers{
						Azure: &modelv20250801.ProvidersAzure{
							SubscriptionID: to.Ptr("test-subscription-id"),
						},
						Kubernetes: &modelv20250801.ProvidersKubernetes{
							Namespace: to.Ptr(envNamespace),
						},
					},
					Simulated: to.Ptr(false),
				},
			},
			appResource: nil,
			expectedConfig: &recipes.Configuration{
				Runtime: recipes.RuntimeConfiguration{
					Kubernetes: &recipes.KubernetesRuntime{
						Namespace:            envNamespace,
						EnvironmentNamespace: envNamespace,
					},
				},
				Providers: datamodel.Providers{
					Azure: datamodel.ProvidersAzure{
						Scope: "test-subscription-id",
					},
				},
				Simulated: false,
			},
		},
		{
			name: "aws provider with env resource v20250801",
			envResource: &modelv20250801.EnvironmentResource{
				Properties: &modelv20250801.EnvironmentProperties{
					Providers: &modelv20250801.Providers{
						Aws: &modelv20250801.ProvidersAws{
							Scope: to.Ptr(awsScope),
						},
						Kubernetes: &modelv20250801.ProvidersKubernetes{
							Namespace: to.Ptr(envNamespace),
						},
					},
					Simulated: to.Ptr(false),
				},
			},
			appResource: nil,
			expectedConfig: &recipes.Configuration{
				Runtime: recipes.RuntimeConfiguration{
					Kubernetes: &recipes.KubernetesRuntime{
						Namespace:            envNamespace,
						EnvironmentNamespace: envNamespace,
					},
				},
				Providers: datamodel.Providers{
					AWS: datamodel.ProvidersAWS{
						Scope: awsScope,
					},
				},
				Simulated: false,
			},
		},
		{
			name: "simulated env v20250801",
			envResource: &modelv20250801.EnvironmentResource{
				Properties: &modelv20250801.EnvironmentProperties{
					Providers: &modelv20250801.Providers{
						Kubernetes: &modelv20250801.ProvidersKubernetes{
							Namespace: to.Ptr(envNamespace),
						},
					},
					Simulated: to.Ptr(true),
				},
			},
			appResource: nil,
			expectedConfig: &recipes.Configuration{
				Runtime: recipes.RuntimeConfiguration{
					Kubernetes: &recipes.KubernetesRuntime{
						Namespace:            envNamespace,
						EnvironmentNamespace: envNamespace,
					},
				},
				Simulated: true,
			},
		},
		{
			name: "environment with recipe packs v20250801",
			envResource: &modelv20250801.EnvironmentResource{
				Properties: &modelv20250801.EnvironmentProperties{
					Providers: &modelv20250801.Providers{
						Kubernetes: &modelv20250801.ProvidersKubernetes{
							Namespace: to.Ptr(envNamespace),
						},
					},
					RecipePacks: []*string{
						to.Ptr("/planes/radius/local/providers/Radius.Core/recipePacks/kubernetes-pack"),
					},
				},
			},
			appResource: nil,
			expectedConfig: &recipes.Configuration{
				Runtime: recipes.RuntimeConfiguration{
					Kubernetes: &recipes.KubernetesRuntime{
						Namespace:            envNamespace,
						EnvironmentNamespace: envNamespace,
					},
				},
				Simulated: false,
			},
		},
	}

	for _, tc := range configTests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := getConfigurationV20250801(tc.envResource)
			if tc.errString != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errString)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedConfig, result)
			}
		})
	}
}

func TestGetRecipeDefinitionFromEnvironmentV20250801(t *testing.T) {
	ctx := context.Background()
	armOptions := &arm.ClientOptions{}

	envResource := &modelv20250801.EnvironmentResource{
		Properties: &modelv20250801.EnvironmentProperties{
			Providers: &modelv20250801.Providers{
				Kubernetes: &modelv20250801.ProvidersKubernetes{
					Namespace: to.Ptr(envNamespace),
				},
			},
			RecipePacks: []*string{
				to.Ptr("/planes/radius/local/providers/Radius.Core/recipePacks/kubernetes-pack"),
			},
		},
	}

	recipeMetadata := recipes.ResourceMetadata{
		Name:          recipeName,
		EnvironmentID: envResourceId,
		ResourceID:    mongoResourceID,
	}

	t.Run("invalid resource id", func(t *testing.T) {
		metadata := recipeMetadata
		metadata.ResourceID = "invalid-id"
		_, err := getRecipeDefinitionFromEnvironmentV20250801(ctx, envResource, &metadata, armOptions)
		require.Error(t, err)
		require.Contains(t, err.Error(), "'invalid-id' is not a valid resource id")
	})
}

func Test_reconcileRecipeParameters(t *testing.T) {
	tests := []struct {
		name             string
		recipePackParams map[string]any
		envRecipeParams  map[string]map[string]any
		resourceType     string
		expected         map[string]any
	}{
		{
			name: "no environment parameters - returns recipe pack parameters",
			recipePackParams: map[string]any{
				"param1": "value1",
				"param2": 42,
			},
			envRecipeParams: nil,
			resourceType:    "Radius.Compute/containers",
			expected: map[string]any{
				"param1": "value1",
				"param2": 42,
			},
		},
		{
			name: "environment parameters override recipe pack parameters",
			recipePackParams: map[string]any{
				"param1": "originalValue",
				"param2": 42,
			},
			envRecipeParams: map[string]map[string]any{
				"Radius.Compute/containers": {
					"param1": "overriddenValue",
				},
			},
			resourceType: "Radius.Compute/containers",
			expected: map[string]any{
				"param1": "overriddenValue",
				"param2": 42,
			},
		},
		{
			name: "environment parameters add new parameters",
			recipePackParams: map[string]any{
				"param1": "value1",
			},
			envRecipeParams: map[string]map[string]any{
				"Radius.Compute/containers": {
					"param2": "value2",
					"param3": true,
				},
			},
			resourceType: "Radius.Compute/containers",
			expected: map[string]any{
				"param1": "value1",
				"param2": "value2",
				"param3": true,
			},
		},
		{
			name: "environment parameters for different resource type - no override",
			recipePackParams: map[string]any{
				"param1": "value1",
			},
			envRecipeParams: map[string]map[string]any{
				"Radius.Data/postgreSQL": {
					"param2": "value2",
				},
			},
			resourceType: "Radius.Compute/containers",
			expected: map[string]any{
				"param1": "value1",
			},
		},
		{
			name:             "empty recipe pack parameters with environment parameters",
			recipePackParams: map[string]any{},
			envRecipeParams: map[string]map[string]any{
				"Radius.Compute/containers": {
					"param1": "value1",
					"param2": 42,
				},
			},
			resourceType: "Radius.Compute/containers",
			expected: map[string]any{
				"param1": "value1",
				"param2": 42,
			},
		},
		{
			name:             "nil recipe pack parameters with environment parameters",
			recipePackParams: nil,
			envRecipeParams: map[string]map[string]any{
				"Radius.Compute/containers": {
					"param1": "value1",
				},
			},
			resourceType: "Radius.Compute/containers",
			expected: map[string]any{
				"param1": "value1",
			},
		},
		{
			name: "empty environment parameters map",
			recipePackParams: map[string]any{
				"param1": "value1",
			},
			envRecipeParams: map[string]map[string]any{},
			resourceType:    "Radius.Compute/containers",
			expected: map[string]any{
				"param1": "value1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := reconcileRecipeParameters(tt.recipePackParams, tt.envRecipeParams, tt.resourceType)
			require.Equal(t, tt.expected, result)
		})
	}
}
