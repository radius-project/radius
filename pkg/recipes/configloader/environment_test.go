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
	"testing"

	model "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
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
