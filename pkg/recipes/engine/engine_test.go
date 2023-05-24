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

package engine

import (
	"context"
	"testing"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/configloader"
	"github.com/project-radius/radius/pkg/recipes/driver"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	"github.com/stretchr/testify/require"
)

func createContext(t *testing.T) context.Context {
	logger, err := ucplog.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func setup(t *testing.T) (engine, configloader.MockConfigurationLoader, driver.MockDriver) {
	ctrl := gomock.NewController(t)
	configLoader := configloader.NewMockConfigurationLoader(ctrl)
	mDriver := driver.NewMockDriver(ctrl)
	options := Options{
		ConfigurationLoader: configLoader,
		Drivers: map[string]driver.Driver{
			"bicep": mDriver,
		},
	}
	engine := engine{
		options: options,
	}
	return engine, *configLoader, *mDriver
}

func Test_Engine_Success(t *testing.T) {
	recipeMetadata := recipes.Metadata{
		Name:          "mongo-azure",
		ApplicationID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/app1",
		EnvironmentID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/environments/env1",
		ResourceID:    "/planes/deployments/local/resourceGroups/test-rg/providers/Microsoft.Resources/deployments/recipe",
		Parameters: map[string]any{
			"resourceName": "resource1",
		},
	}
	envConfig := &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace: "default",
			},
		},
		Providers: datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: "scope",
			},
		},
	}
	recipeResult := &recipes.RecipeOutput{
		Resources: []string{"mongoStorageAccount", "mongoDatabase"},
		Secrets: map[string]any{
			"connectionString": "mongodb://testUser:testPassword@testAccount1.mongo.cosmos.azure.com:10255",
		},
		Values: map[string]any{
			"host": "testAccount1.mongo.cosmos.azure.com",
			"port": 10255,
		},
	}
	recipeDefinition := &recipes.Definition{
		Driver:       recipes.TemplateKindBicep,
		TemplatePath: "radiusdev.azurecr.io/recipes/functionaltest/basic/mongodatabases/azure:1.0",
		ResourceType: "Applications.Link/mongoDatabases",
	}
	ctx := createContext(t)
	engine, configLoader, driver := setup(t)

	configLoader.EXPECT().LoadConfiguration(gomock.Any(), gomock.Any()).Times(1).Return(envConfig, nil)
	configLoader.EXPECT().LoadRecipe(gomock.Any(), gomock.Any()).Times(1).Return(recipeDefinition, nil)
	driver.EXPECT().Execute(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(recipeResult, nil)

	result, err := engine.Execute(ctx, recipeMetadata)
	require.NoError(t, err)
	require.Equal(t, result, recipeResult)
}

func Test_Engine_InvalidDriver(t *testing.T) {
	ctx := createContext(t)
	engine, configLoader, _ := setup(t)

	recipeDefinition := &recipes.Definition{
		Driver:       "invalid",
		TemplatePath: "radiusdev.azurecr.io/recipes/functionaltest/basic/mongodatabases/azure:1.0",
		ResourceType: "Applications.Link/mongoDatabases",
	}

	recipeMetadata := recipes.Metadata{
		Name:          "mongo-azure",
		ApplicationID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/app1",
		EnvironmentID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/environments/env1",
		ResourceID:    "/planes/deployments/local/resourceGroups/test-rg/providers/Microsoft.Resources/deployments/recipe",
		Parameters: map[string]any{
			"resourceName": "resource1",
		},
	}

	configLoader.EXPECT().LoadRecipe(gomock.Any(), gomock.Any()).Times(1).Return(recipeDefinition, nil)
	_, err := engine.Execute(ctx, recipeMetadata)
	require.Error(t, err)
	require.Equal(t, err.Error(), "could not find driver invalid")
}

func Test_Engine_Lookup_Error(t *testing.T) {
	ctx := createContext(t)
	engine, configLoader, _ := setup(t)
	recipeMetadata := recipes.Metadata{
		Name:          "mongo-azure",
		ApplicationID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/app1",
		EnvironmentID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/environments/env1",
		ResourceID:    "/planes/deployments/local/resourceGroups/test-rg/providers/Microsoft.Resources/deployments/recipe",
		Parameters: map[string]any{
			"resourceName": "resource1",
		},
	}
	configLoader.EXPECT().LoadRecipe(gomock.Any(), gomock.Any()).Times(1).Return(nil, errors.New("could not find recipe mongo-azure in environment env1"))
	_, err := engine.Execute(ctx, recipeMetadata)
	require.Error(t, err)
}

func Test_Engine_Load_Error(t *testing.T) {
	ctx := createContext(t)
	engine, configLoader, _ := setup(t)
	recipeMetadata := recipes.Metadata{
		Name:          "mongo-azure",
		ApplicationID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/app1",
		EnvironmentID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/environments/env1",
		ResourceID:    "/planes/deployments/local/resourceGroups/test-rg/providers/Microsoft.Resources/deployments/recipe",
		Parameters: map[string]any{
			"resourceName": "resource1",
		},
	}
	recipeDefinition := &recipes.Definition{
		Driver:       recipes.TemplateKindBicep,
		TemplatePath: "radiusdev.azurecr.io/recipes/functionaltest/basic/mongodatabases/azure:1.0",
		ResourceType: "Applications.Link/mongoDatabases",
	}
	configLoader.EXPECT().LoadRecipe(gomock.Any(), gomock.Any()).Times(1).Return(recipeDefinition, nil)
	configLoader.EXPECT().LoadConfiguration(gomock.Any(), gomock.Any()).Times(1).Return(nil, errors.New("unable to fetch namespace information"))
	_, err := engine.Execute(ctx, recipeMetadata)
	require.Error(t, err)
}
