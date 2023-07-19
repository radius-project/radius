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
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	gomock "github.com/golang/mock/gomock"
	"github.com/google/uuid"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/terraform"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

func setup(t *testing.T) (terraform.MockTerraformExecutor, terraformDriver) {
	ctrl := gomock.NewController(t)
	tfExecutor := terraform.NewMockTerraformExecutor(ctrl)

	driver := terraformDriver{tfExecutor, TerraformOptions{Path: t.TempDir()}}

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
		ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/applications.link/rediscaches/test-redis-recipe",
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
	ctx := testcontext.New(t)
	armCtx := &v1.ARMRequestContext{
		OperationID: uuid.New(),
	}
	ctx = v1.WithARMRequestContext(ctx, armCtx)

	tfExecutor, driver := setup(t)
	envConfig, recipeMetadata, envRecipe := buildTestInputs()
	tfDir := filepath.Join(driver.options.Path, armCtx.OperationID.String())
	options := terraform.Options{
		RootDir:        tfDir,
		EnvConfig:      &envConfig,
		ResourceRecipe: &recipeMetadata,
		EnvRecipe:      &envRecipe,
	}
	expectedOutput := &recipes.RecipeOutput{
		Values: map[string]any{
			"host": "myrediscache.redis.cache.windows.net",
			"port": json.Number("6379"),
		},
	}

	tfExecutor.EXPECT().Deploy(ctx, options).Times(1).Return(expectedOutput, nil)

	recipeOutput, err := driver.Execute(ctx, envConfig, recipeMetadata, envRecipe)
	require.Error(t, err)
	require.Equal(t, "terraform support is not implemented yet", err.Error())
	require.Equal(t, expectedOutput, recipeOutput)
	// Verify directory cleanup
	_, err = os.Stat(tfDir)
	require.True(t, os.IsNotExist(err), "Expected directory %s to be removed, but it still exists", tfDir)
}

func TestTerraformDriver_Execute_DeploymentFailure(t *testing.T) {
	ctx := testcontext.New(t)
	armCtx := &v1.ARMRequestContext{
		OperationID: uuid.New(),
	}
	ctx = v1.WithARMRequestContext(ctx, armCtx)

	tfExecutor, driver := setup(t)
	envConfig, recipeMetadata, envRecipe := buildTestInputs()
	tfDir := filepath.Join(driver.options.Path, armCtx.OperationID.String())
	options := terraform.Options{
		RootDir:        tfDir,
		EnvConfig:      &envConfig,
		ResourceRecipe: &recipeMetadata,
		EnvRecipe:      &envRecipe,
	}

	tfExecutor.EXPECT().Deploy(ctx, options).Times(1).Return(nil, errors.New("Failed to deploy terraform module"))

	_, err := driver.Execute(ctx, envConfig, recipeMetadata, envRecipe)
	require.Error(t, err)
	require.Equal(t, "Failed to deploy terraform module", err.Error())
	// Verify directory cleanup
	_, err = os.Stat(tfDir)
	require.True(t, os.IsNotExist(err), "Expected directory %s to be removed, but it still exists", tfDir)
}

func TestTerraformDriver_Execute_EmptyPath(t *testing.T) {
	_, driver := setup(t)
	driver.options.Path = ""
	envConfig, recipeMetadata, envRecipe := buildTestInputs()

	_, err := driver.Execute(testcontext.New(t), envConfig, recipeMetadata, envRecipe)
	require.Error(t, err)
	require.Equal(t, "path is a required option for Terraform driver", err.Error())
}

func TestTerraformDriver_Execute_EmptyOperationID_Success(t *testing.T) {
	ctx := testcontext.New(t)
	ctx = v1.WithARMRequestContext(ctx, &v1.ARMRequestContext{})

	tfExecutor, driver := setup(t)
	envConfig, recipeMetadata, envRecipe := buildTestInputs()
	expectedOutput := &recipes.RecipeOutput{
		Values: map[string]any{
			"host": "myrediscache.redis.cache.windows.net",
			"port": json.Number("6379"),
		},
	}

	tfExecutor.EXPECT().Deploy(ctx, gomock.Any()).Times(1).Return(expectedOutput, nil)

	recipeOutput, err := driver.Execute(ctx, envConfig, recipeMetadata, envRecipe)
	require.Error(t, err)
	require.Equal(t, "terraform support is not implemented yet", err.Error())
	require.Equal(t, expectedOutput, recipeOutput)
}

func TestTerraformDriver_Execute_InvalidContextPanics(t *testing.T) {
	ctx := testcontext.New(t)

	_, driver := setup(t)
	envConfig, recipeMetadata, envRecipe := buildTestInputs()

	require.Panics(t, func() {
		_, _ = driver.Execute(ctx, envConfig, recipeMetadata, envRecipe)
	})
}

func TestTerraformDriver_Delete_Success(t *testing.T) {
	ctx := testcontext.New(t)

	_, driver := setup(t)

	err := driver.Delete(ctx, []rpv1.OutputResource{})
	require.Error(t, err)
}
