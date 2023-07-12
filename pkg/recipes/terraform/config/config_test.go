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

package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

const (
	testTemplatePath    = "Azure/redis/azurerm"
	testRecipeName      = "redis-azure"
	testTemplateVersion = "1.1.0"
)

var (
	envParams = map[string]any{
		"resource_group_name": "test-rg",
		"sku":                 "C",
	}

	resourceParams = map[string]any{
		"redis_cache_name": "redis-test",
		"sku":              "P",
	}
)

func TestGenerateMainConfigFile(t *testing.T) {
	// Create a temporary directory for testing.
	testDir := t.TempDir()

	envRecipe := recipes.EnvironmentDefinition{
		Name:            testRecipeName,
		TemplatePath:    testTemplatePath,
		TemplateVersion: testTemplateVersion,
		Parameters:      envParams,
	}

	resourceRecipe := recipes.ResourceMetadata{
		Name:       testRecipeName,
		Parameters: resourceParams,
	}

	err := GenerateMainConfigFile(testcontext.New(t), &envRecipe, &resourceRecipe, testDir, testRecipeName)
	require.NoError(t, err)

	// Assert that the main.tf.json file was created.
	configFilePath := filepath.Join(testDir, mainConfigFileName)
	_, err = os.Stat(configFilePath)
	require.NoError(t, err)

	// Read the JSON data from the main.tf.json file.
	jsonData, err := os.ReadFile(configFilePath)
	require.NoError(t, err)

	// Unmarshal the JSON data into a TerraformConfig struct.
	var tfConfig TerraformConfig
	err = json.Unmarshal(jsonData, &tfConfig)
	require.NoError(t, err)

	// Assert that the TerraformConfig struct contains the expected data.
	expectedTfConfig := TerraformConfig{
		Module: map[string]any{
			testRecipeName: map[string]any{
				moduleSourceKey:       testTemplatePath,
				moduleVersionKey:      testTemplateVersion,
				"resource_group_name": envParams["resource_group_name"],
				"redis_cache_name":    resourceParams["redis_cache_name"],
				"sku":                 resourceParams["sku"],
			},
		},
	}
	require.Equal(t, expectedTfConfig, tfConfig)
}

func TestGenerateMainConfig_EmptyParameters(t *testing.T) {
	testDir := t.TempDir()
	envRecipe := recipes.EnvironmentDefinition{
		Name:            testRecipeName,
		TemplatePath:    testTemplatePath,
		TemplateVersion: testTemplateVersion,
	}

	resourceRecipe := recipes.ResourceMetadata{
		Name: testRecipeName,
	}

	err := GenerateMainConfigFile(testcontext.New(t), &envRecipe, &resourceRecipe, testDir, testRecipeName)
	require.NoError(t, err)

	// Assert that the main.tf.json file was created.
	configFilePath := filepath.Join(testDir, mainConfigFileName)
	_, err = os.Stat(configFilePath)
	require.NoError(t, err)

	// Read the JSON data from the main.tf.json file.
	jsonData, err := os.ReadFile(configFilePath)
	require.NoError(t, err)

	// Unmarshal the JSON data into a TerraformConfig struct.
	var tfConfig TerraformConfig
	err = json.Unmarshal(jsonData, &tfConfig)
	require.NoError(t, err)

	// Assert that the TerraformConfig struct contains the expected data.
	expectedTfConfig := TerraformConfig{
		Module: map[string]any{
			testRecipeName: map[string]any{
				moduleSourceKey:  testTemplatePath,
				moduleVersionKey: testTemplateVersion,
			},
		},
	}
	require.Equal(t, expectedTfConfig, tfConfig)
}

func TestGenerateMainConfig_Error(t *testing.T) {
	envRecipe := recipes.EnvironmentDefinition{
		TemplatePath:    testTemplatePath,
		TemplateVersion: testTemplateVersion,
		Parameters:      envParams,
	}

	resourceRecipe := recipes.ResourceMetadata{
		Name:       testRecipeName,
		Parameters: resourceParams,
	}

	// Call GenerateMainConfig with a working directory that doesn't exist.
	invalidPath := filepath.Join("invalid", uuid.New().String())
	err := GenerateMainConfigFile(context.Background(), &envRecipe, &resourceRecipe, invalidPath, testRecipeName)
	require.Error(t, err)
	require.Contains(t, err.Error(), "error creating file")
}

func TestGenerateModuleData(t *testing.T) {
	moduleData := generateModuleData(testcontext.New(t), testTemplatePath, testTemplateVersion, envParams, resourceParams)

	// Assert that the module data contains the expected data.
	expectedModuleData := map[string]any{
		moduleSourceKey:       testTemplatePath,
		moduleVersionKey:      testTemplateVersion,
		"resource_group_name": envParams["resource_group_name"],
		"redis_cache_name":    resourceParams["redis_cache_name"],
		"sku":                 resourceParams["sku"],
	}
	require.Equal(t, expectedModuleData, moduleData)
}
