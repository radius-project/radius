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
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/terraform/config/providers"
	"github.com/project-radius/radius/test/testcontext"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
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

func setup(t *testing.T) (providers.MockProvider, map[string]providers.Provider) {
	ctrl := gomock.NewController(t)
	mProvider := providers.NewMockProvider(ctrl)
	providers := map[string]providers.Provider{
		providers.AWSProviderName:        mProvider,
		providers.AzureProviderName:      mProvider,
		providers.KubernetesProviderName: mProvider,
	}

	return *mProvider, providers
}

func getTestInputs() (recipes.EnvironmentDefinition, recipes.ResourceMetadata) {
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

	return envRecipe, resourceRecipe
}

func validateConfigIsGenerated(configFilePath string) (TerraformConfig, error) {
	// Validate that the config file was created.
	_, err := os.Stat(configFilePath)
	if err != nil {
		return TerraformConfig{}, err
	}

	// Read the JSON data from the main.tf.json file.
	jsonData, err := os.ReadFile(configFilePath)
	if err != nil {
		return TerraformConfig{}, err
	}

	// Unmarshal the JSON data into a TerraformConfig struct.
	var tfConfig TerraformConfig
	err = json.Unmarshal(jsonData, &tfConfig)
	if err != nil {
		return TerraformConfig{}, err
	}

	return tfConfig, nil
}

func TestGenerateTFConfigFile(t *testing.T) {
	// Create a temporary test directory.
	testDir := t.TempDir()
	envRecipe, resourceRecipe := getTestInputs()

	expectedTFConfig := TerraformConfig{
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

	configFilePath, err := GenerateTFConfigFile(testcontext.New(t), &envRecipe, &resourceRecipe, testDir, testRecipeName)
	require.NoError(t, err)

	// Assert config file exists and contains data in expected format.
	tfConfig, err := validateConfigIsGenerated(configFilePath)
	require.NoError(t, err)

	// Assert that generated config contains the expected data.
	require.Equal(t, expectedTFConfig, tfConfig)
}

func TestGenerateTFConfig_EmptyParameters(t *testing.T) {
	// Create a temporary test directory.
	testDir := t.TempDir()

	envRecipe, resourceRecipe := getTestInputs()
	envRecipe.Parameters = nil
	resourceRecipe.Parameters = nil

	expectedTFConfig := TerraformConfig{
		Module: map[string]any{
			testRecipeName: map[string]any{
				moduleSourceKey:  testTemplatePath,
				moduleVersionKey: testTemplateVersion,
			},
		},
	}

	configFilePath, err := GenerateTFConfigFile(testcontext.New(t), &envRecipe, &resourceRecipe, testDir, testRecipeName)
	require.NoError(t, err)

	// Assert config file exists and contains data in expected format.
	tfConfig, err := validateConfigIsGenerated(configFilePath)
	require.NoError(t, err)

	// Assert that generated config contains the expected data.
	require.Equal(t, expectedTFConfig, tfConfig)
}

func TestGenerateTFConfig_InvalidWorkingDir_Error(t *testing.T) {
	envRecipe, resourceRecipe := getTestInputs()

	// Call GenerateMainConfig with a working directory that doesn't exist.
	invalidPath := filepath.Join("invalid", uuid.New().String())
	_, err := GenerateTFConfigFile(testcontext.New(t), &envRecipe, &resourceRecipe, invalidPath, testRecipeName)
	require.Error(t, err)
	require.Contains(t, err.Error(), "error creating file")
}

func TestGenerateModuleData(t *testing.T) {
	expectedModuleData := map[string]any{
		moduleSourceKey:       testTemplatePath,
		moduleVersionKey:      testTemplateVersion,
		"resource_group_name": envParams["resource_group_name"],
		"redis_cache_name":    resourceParams["redis_cache_name"],
		"sku":                 resourceParams["sku"],
	}

	moduleData := generateModuleData(testcontext.New(t), testTemplatePath, testTemplateVersion, envParams, resourceParams)

	// Assert that the module data contains the expected data.
	require.Equal(t, expectedModuleData, moduleData)
}

func TestAddProviders_Success(t *testing.T) {
	ctx := testcontext.New(t)
	// Create a temporary test directory.
	testDir := t.TempDir()
	mProvider, supportedProviders := setup(t)
	envRecipe, resourceRecipe := getTestInputs()
	envConfig := recipes.Configuration{
		Providers: datamodel.Providers{
			AWS: datamodel.ProvidersAWS{
				Scope: "/planes/aws/aws/accounts/0000/regions/test-region",
			},
			Azure: datamodel.ProvidersAzure{
				Scope: "/subscriptions/test-sub/resourceGroups/test-rg",
			},
		},
	}

	awsProviderConfig := map[string]any{
		"region": "test-region",
	}
	azureProviderConfig := map[string]any{
		"subscription_id": "test-sub",
		"features":        map[string]any{},
	}
	kubernetesProviderConfig := map[string]any{
		"config_path": clientcmd.RecommendedHomeFile,
	}
	expectedTFConfig := TerraformConfig{
		Provider: map[string]any{
			providers.AWSProviderName:        awsProviderConfig,
			providers.AzureProviderName:      azureProviderConfig,
			providers.KubernetesProviderName: kubernetesProviderConfig,
		},
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

	configFilePath, err := GenerateTFConfigFile(ctx, &envRecipe, &resourceRecipe, testDir, testRecipeName)
	require.NoError(t, err)

	mProvider.EXPECT().BuildConfig(ctx, &envConfig).Times(1).Return(awsProviderConfig, nil)
	mProvider.EXPECT().BuildConfig(ctx, &envConfig).Times(1).Return(azureProviderConfig, nil)
	mProvider.EXPECT().BuildConfig(ctx, &envConfig).Times(1).Return(kubernetesProviderConfig, nil)

	err = AddProviders(ctx, configFilePath, []string{providers.AWSProviderName, providers.AzureProviderName, providers.KubernetesProviderName, "sql"}, supportedProviders, &envConfig)
	require.NoError(t, err)

	// Validate that the config file exists and read the updated data.
	tfConfig, err := validateConfigIsGenerated(configFilePath)
	require.NoError(t, err)
	// Assert that the TerraformConfig contains the expected data.
	require.Equal(t, expectedTFConfig, tfConfig)
}

func TestAddProviders_EmptyAWSScope(t *testing.T) {
	ctx := testcontext.New(t)
	// Create a temporary test directory.
	testDir := t.TempDir()
	mProvider, supportedProviders := setup(t)
	envRecipe, resourceRecipe := getTestInputs()
	envConfig := recipes.Configuration{
		Providers: datamodel.Providers{
			AWS: datamodel.ProvidersAWS{
				Scope: "",
			},
			Azure: datamodel.ProvidersAzure{
				Scope: "/subscriptions/test-sub/resourceGroups/test-rg",
			},
		},
	}

	configFilePath, err := GenerateTFConfigFile(ctx, &envRecipe, &resourceRecipe, testDir, testRecipeName)
	require.NoError(t, err)

	mProvider.EXPECT().BuildConfig(ctx, &envConfig).Times(1).Return(nil, errors.New("AWS provider is required to be configured on the Environment to create AWS resources using Recipe"))

	err = AddProviders(ctx, configFilePath, []string{providers.AWSProviderName}, supportedProviders, &envConfig)
	require.Error(t, err)
	require.ErrorContains(t, err, "AWS provider is required to be configured on the Environment to create AWS resources using Recipe")
}

func TestAddProviders_MissingAzureProvider(t *testing.T) {
	ctx := testcontext.New(t)
	// Create a temporary test directory.
	testDir := t.TempDir()
	mProvider, supportedProviders := setup(t)
	envRecipe, resourceRecipe := getTestInputs()

	configFilePath, err := GenerateTFConfigFile(ctx, &envRecipe, &resourceRecipe, testDir, testRecipeName)
	require.NoError(t, err)

	mProvider.EXPECT().BuildConfig(ctx, &recipes.Configuration{}).Times(1).Return(nil, errors.New("Azure provider is required to be configured on the Environment to create Azure resources using Recipe"))

	err = AddProviders(ctx, configFilePath, []string{providers.AzureProviderName}, supportedProviders, &recipes.Configuration{})
	require.Error(t, err)
	require.ErrorContains(t, err, "Azure provider is required to be configured on the Environment to create Azure resources using Recipe")
}

func TestAddProviders_OpenFileError(t *testing.T) {
	ctx := testcontext.New(t)
	mProvider, supportedProviders := setup(t)
	kubernetesProviderConfig := map[string]any{
		"config_path": clientcmd.RecommendedHomeFile,
	}

	mProvider.EXPECT().BuildConfig(ctx, nil).Times(1).Return(kubernetesProviderConfig, nil)

	// Call AddProviders with a non-existent file path.
	err := AddProviders(ctx, "/path/to/non-existent/file.json", []string{providers.KubernetesProviderName}, supportedProviders, nil)

	// Assert that AddProviders returns an error.
	require.Error(t, err)
	require.Contains(t, err.Error(), "no such file or directory")
}

func TestAddProviders_DecodeError(t *testing.T) {
	ctx := testcontext.New(t)
	mProvider, supportedProviders := setup(t)
	// Create a temporary test directory.
	testDir := t.TempDir()
	// Create a test configuration file with invalid JSON data.
	configFile := filepath.Join(testDir, "test.json")
	err := os.WriteFile(configFile, []byte(`invalid json data`), 0644)
	require.NoError(t, err)

	kubernetesProviderConfig := map[string]any{
		"config_path": clientcmd.RecommendedHomeFile,
	}
	mProvider.EXPECT().BuildConfig(ctx, nil).Times(1).Return(kubernetesProviderConfig, nil)

	// Call AddProviders with the test configuration file and required providers.
	err = AddProviders(ctx, configFile, []string{providers.KubernetesProviderName}, supportedProviders, nil)

	// Assert that AddProviders returns an error.
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid character")
}

func TestAddProviders_WriteFileError(t *testing.T) {
	ctx := testcontext.New(t)
	mProvider, supportedProviders := setup(t)
	// Create a temporary test directory.
	testDir := t.TempDir()

	// Create a test configuration file.
	configFile := filepath.Join(testDir, "test.json")
	err := os.WriteFile(configFile, []byte(`{"module":{}}`), 0644)
	require.NoError(t, err)
	// Mock a write file error by setting the file permissions to read-only.
	err = os.Chmod(configFile, 0400)
	require.NoError(t, err)

	kubernetesProviderConfig := map[string]any{
		"config_path": clientcmd.RecommendedHomeFile,
	}
	mProvider.EXPECT().BuildConfig(ctx, nil).Times(1).Return(kubernetesProviderConfig, nil)

	// Call AddProviders with the test configuration file and required providers.
	err = AddProviders(ctx, configFile, []string{providers.KubernetesProviderName}, supportedProviders, nil)

	// Assert that AddProviders returns an error.
	require.Error(t, err)
	require.Contains(t, err.Error(), "permission denied")
}
