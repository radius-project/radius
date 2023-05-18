// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package configloader

import (
	"testing"

	model "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

const (
	kind            = "kubernetes"
	namespace       = "default"
	appNamespace    = "app-default"
	envResourceId   = "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0"
	appResourceId   = "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/app0"
	azureScope      = "/subscriptions/test-sub/resourceGroups/testRG"
	awsScope        = "/planes/aws/aws/accounts/000/regions/cool-region"
	mongoResourceID = "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Link/mongoDatabases/mongo-database-0"
	redisID         = "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Link/redisCaches/redis-0"
)

func Test_GetConfigurationAzure(t *testing.T) {
	envConfig := &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				EnvironmentNamespace: "default",
			},
		},
		Providers: createAzureProvider(),
	}
	envResource := model.EnvironmentResource{
		Properties: &model.EnvironmentProperties{
			Compute: &model.KubernetesCompute{
				Kind:       to.Ptr(kind),
				Namespace:  to.Ptr(namespace),
				ResourceID: to.Ptr(envResourceId),
			},
			Providers: &model.Providers{
				Azure: &model.ProvidersAzure{
					Scope: to.Ptr(azureScope),
				},
			},
		},
	}
	result, err := getConfiguration(&envResource, nil)
	require.NoError(t, err)
	require.Equal(t, envConfig, result)
}

func Test_GetConfigurationAWS(t *testing.T) {
	envConfig := &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				EnvironmentNamespace: "default",
			},
		},
		Providers: createAWSProvider(),
	}
	envResource := model.EnvironmentResource{
		Properties: &model.EnvironmentProperties{
			Compute: &model.KubernetesCompute{
				Kind:       to.Ptr(kind),
				Namespace:  to.Ptr(namespace),
				ResourceID: to.Ptr(envResourceId),
			},
			Providers: &model.Providers{
				Aws: &model.ProvidersAws{
					Scope: to.Ptr(awsScope),
				},
			},
		},
	}
	result, err := getConfiguration(&envResource, nil)
	require.NoError(t, err)
	require.Equal(t, envConfig, result)

	appConfig := &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace: "app-default",
			},
		},
		Providers: createAWSProvider(),
	}
	appResource := model.ApplicationResource{
		Properties: &model.ApplicationProperties{
			Status: &model.ResourceStatus{
				Compute: &model.KubernetesCompute{
					Kind:       to.Ptr(kind),
					Namespace:  to.Ptr(appNamespace),
					ResourceID: to.Ptr(appResourceId),
				},
			},
		},
	}
	result, err = getConfiguration(&envResource, &appResource)
	require.NoError(t, err)
	require.Equal(t, appConfig, result)
}

func Test_InvalidApplicationError(t *testing.T) {
	envResource := model.EnvironmentResource{
		Properties: &model.EnvironmentProperties{
			Compute: &model.KubernetesCompute{
				Kind:       to.Ptr(kind),
				Namespace:  to.Ptr(namespace),
				ResourceID: to.Ptr(envResourceId),
			},
		},
	}
	// Invalid app model (should have KubernetesCompute field)
	appResource := model.ApplicationResource{
		Properties: &model.ApplicationProperties{
			Status: &model.ResourceStatus{
				Compute: &model.EnvironmentCompute{},
			},
		},
	}
	_, err := getConfiguration(&envResource, &appResource)
	require.Error(t, err)
	require.Equal(t, err.Error(), "invalid model conversion")
}

func Test_InvalidEnvError(t *testing.T) {
	// Invalid env model (should have KubernetesCompute field)
	envResource := model.EnvironmentResource{
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
	}
	_, err := getConfiguration(&envResource, nil)
	require.Error(t, err)
	require.Equal(t, err.Error(), "invalid model conversion")
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
				Namespace:  to.Ptr(namespace),
				ResourceID: to.Ptr(envResourceId),
			},
			Providers: &model.Providers{
				Azure: &model.ProvidersAzure{
					Scope: to.Ptr(azureScope),
				},
			},
			Recipes: map[string]map[string]*model.EnvironmentRecipeProperties{
				"Applications.Link/mongoDatabases": {
					"cosmosDB": {
						TemplateKind: to.Ptr(recipes.TemplateKindBicep),
						TemplatePath: to.Ptr("radiusdev.azurecr.io/recipes/mongodatabases/azure:1.0"),
						Parameters: map[string]any{
							"foo": "bar",
						},
					},
					"default": {
						TemplateKind: to.Ptr(recipes.TemplateKindBicep),
						TemplatePath: to.Ptr("radiusdev.azurecr.io/recipes/mongoDefault/azure:1.0"),
					},
				},
			},
		},
	}
	recipeMetadata := recipes.Metadata{
		Name:          "cosmosDB",
		EnvironmentID: envResourceId,
		ResourceID:    mongoResourceID,
	}
	t.Run("invalid resource id", func(t *testing.T) {
		metadata := recipeMetadata
		metadata.ResourceID = "invalid-id"
		_, err := getRecipeDefinition(&envResource, metadata)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to parse resourceID")
	})
	t.Run("empty recipe name", func(t *testing.T) {
		metadata := recipeMetadata
		metadata.Name = ""
		_, err := getRecipeDefinition(&envResource, metadata)
		require.NoError(t, err)
	})
	t.Run("recipe not found", func(t *testing.T) {
		metadata := recipeMetadata
		metadata.ResourceID = redisID
		_, err := getRecipeDefinition(&envResource, metadata)
		require.Error(t, err)
		require.Contains(t, err.Error(), "could not find recipe")
	})
	t.Run("success", func(t *testing.T) {
		expected := recipes.Definition{
			Driver:       recipes.TemplateKindBicep,
			ResourceType: "Applications.Link/mongoDatabases",
			TemplatePath: "radiusdev.azurecr.io/recipes/mongodatabases/azure:1.0",
			Parameters: map[string]any{
				"foo": "bar",
			},
		}
		recipeDef, err := getRecipeDefinition(&envResource, recipeMetadata)
		require.NoError(t, err)
		require.Equal(t, recipeDef, &expected)
	})
	t.Run("", func(t *testing.T) {
		envResourceNilRecipe := envResource
		envResourceNilRecipe.Properties.Recipes = nil
		_, err := getRecipeDefinition(&envResourceNilRecipe, recipeMetadata)
		require.Error(t, err)
		require.Contains(t, err.Error(), "could not find recipe")
	})

}
