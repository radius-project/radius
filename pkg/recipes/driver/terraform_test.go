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

package driver

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	gomock "github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/terraform"
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

func setup(t *testing.T) (terraform.MockTerraformExecutor, terraformDriver) {
	ctrl := gomock.NewController(t)
	tfExecutor := terraform.NewMockTerraformExecutor(ctrl)

	driver := terraformDriver{tfExecutor, nil, "/tmp"}

	return *tfExecutor, driver
}

func buildTestInputs() (recipes.Configuration, recipes.ResourceMetadata, recipes.EnvironmentDefinition) {
	envConfig := recipes.Configuration{
		Providers: datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: "scope",
			},
		},
	}

	recipeMetadata := recipes.ResourceMetadata{
		Name:          "redis-azure",
		ApplicationID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/app1",
		EnvironmentID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/environments/env1",
		ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Link/redisCaches/test-redis-recipe",
		Parameters: map[string]any{
			"redis_cache_name": "redis-test",
		},
	}

	envRecipe := recipes.EnvironmentDefinition{
		Driver:       recipes.TemplateKindBicep,
		TemplatePath: "Azure/redis/azurerm",
		ResourceType: "Applications.Link/redisCaches",
	}

	return envConfig, recipeMetadata, envRecipe
}

func TestTerraformDriver_Execute_Success(t *testing.T) {
	ctx := createContext(t)
	tfExecutor, driver := setup(t)

	envConfig, recipeMetadata, envRecipe := buildTestInputs()

	expectedOutput := &recipes.RecipeOutput{
		Values: map[string]any{
			"host": "myrediscache.redis.cache.windows.net",
			"port": json.Number("6379"),
		},
	}

	tfExecutor.EXPECT().Deploy(gomock.Any(), gomock.Any()).Times(1).Return(expectedOutput, nil)

	recipeOutput, err := driver.Execute(ctx, envConfig, recipeMetadata, envRecipe)
	require.NoError(t, err, "Expected error to be nil")
	require.Equal(t, expectedOutput, recipeOutput)
}

func TestTerraformDriver_Execute_DeploymentFailure(t *testing.T) {
	ctx := createContext(t)
	tfExecutor, driver := setup(t)

	envConfig, recipeMetadata, envRecipe := buildTestInputs()

	tfExecutor.EXPECT().Deploy(gomock.Any(), gomock.Any()).Times(1).Return(nil, errors.New("Failed to deploy terraform module"))

	_, err := driver.Execute(ctx, envConfig, recipeMetadata, envRecipe)
	require.Error(t, err)
	require.Equal(t, "Failed to deploy terraform module", err.Error())
}

func TestCleanup(t *testing.T) {
	// Create a temporary test directory
	testDir := t.TempDir()

	// Create a new directory under the test directory to ensure that the cleanup function works with subdirectories
	subDir := filepath.Join(testDir, "subdir")
	err := os.Mkdir(subDir, 0755)
	require.NoError(t, err, "Failed to create subdirectory")

	ctx := createContext(t)
	cleanup(ctx, testDir)

	// Verify cleanup
	_, err = os.Stat(testDir)
	require.True(t, os.IsNotExist(err), "Expected directory %s to be removed, but it still exists", testDir)
}
