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
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/recipecontext"
	"github.com/project-radius/radius/pkg/recipes/terraform/config/providers"
	"github.com/project-radius/radius/test/testcontext"
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

func getTestContext() *recipecontext.Context {
	return &recipecontext.Context{
		Resource: recipecontext.Resource{
			ResourceInfo: recipecontext.ResourceInfo{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0",
				Name: "mongo0",
			},
			Type: "applications.link/mongodatabases",
		},
		Application: recipecontext.ResourceInfo{
			Name: "testApplication",
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
		},
		Environment: recipecontext.ResourceInfo{
			Name: "env0",
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
		},
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace:            "radius-test-app",
				EnvironmentNamespace: "radius-test-env",
			},
		},
	}
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

func TestAddRecipeContext(t *testing.T) {
	configTests := []struct {
		name               string
		configPath         string
		envdef             *recipes.EnvironmentDefinition
		metadata           *recipes.ResourceMetadata
		recipeContext      *recipecontext.Context
		expectedConfigFile string
		err                string
	}{
		{
			name: "valid config",
			envdef: &recipes.EnvironmentDefinition{
				Name:            testRecipeName,
				TemplatePath:    testTemplatePath,
				TemplateVersion: testTemplateVersion,
				Parameters:      envParams,
			},
			metadata: &recipes.ResourceMetadata{
				Name:       testRecipeName,
				Parameters: resourceParams,
			},
			recipeContext:      getTestContext(),
			expectedConfigFile: "testdata/main.tf-valid.json",
		},
		{
			name: "without environment definition and metadata params",
			envdef: &recipes.EnvironmentDefinition{
				Name:            testRecipeName,
				TemplatePath:    testTemplatePath,
				TemplateVersion: testTemplateVersion,
			},
			metadata: &recipes.ResourceMetadata{
				Name: testRecipeName,
			},
			recipeContext:      getTestContext(),
			expectedConfigFile: "testdata/main.tf-noparams.json",
		},
		{
			name: "without metadata params",
			envdef: &recipes.EnvironmentDefinition{
				Name:            testRecipeName,
				TemplatePath:    testTemplatePath,
				TemplateVersion: testTemplateVersion,
				Parameters:      envParams,
			},
			metadata: &recipes.ResourceMetadata{
				Name: testRecipeName,
			},
			recipeContext:      getTestContext(),
			expectedConfigFile: "testdata/main.tf-noresourceparam.json",
		},
		{
			name: "without context",
			envdef: &recipes.EnvironmentDefinition{
				Name:            testRecipeName,
				TemplatePath:    testTemplatePath,
				TemplateVersion: testTemplateVersion,
				Parameters:      envParams,
			},
			metadata: &recipes.ResourceMetadata{
				Name:       testRecipeName,
				Parameters: resourceParams,
			},
			recipeContext:      nil,
			expectedConfigFile: "testdata/main.tf-nocontext.json",
		},
		{
			name: "invalid working dir",
			envdef: &recipes.EnvironmentDefinition{
				Name:            testRecipeName,
				TemplatePath:    testTemplatePath,
				TemplateVersion: testTemplateVersion,
				Parameters:      envParams,
			},
			metadata: &recipes.ResourceMetadata{
				Name:       testRecipeName,
				Parameters: resourceParams,
			},
			configPath: filepath.Join("invalid", uuid.New().String()),
			err:        "error creating file: open invalid/",
		},
	}

	for _, tc := range configTests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := testcontext.New(t)
			if tc.configPath == "" {
				tc.configPath = t.TempDir()
			}
			tfconfig := New(testRecipeName, tc.configPath, tc.envdef, tc.metadata)
			if tc.recipeContext != nil {
				err := tfconfig.AddRecipeContext(ctx, tc.recipeContext)
				require.NoError(t, err)
			}
			err := tfconfig.Save(ctx)
			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				return
			}

			require.NoError(t, err)

			// assert
			actualConfig, err := os.ReadFile(tfconfig.ConfigFilePath())
			require.NoError(t, err)
			expectedConfig, err := os.ReadFile(tc.expectedConfigFile)
			require.NoError(t, err)
			require.Equal(t, string(expectedConfig), string(actualConfig))
		})
	}
}

func TestGenerateModuleData(t *testing.T) {
	t.Run("With templateVersion", func(t *testing.T) {
		expectedModuleData := TFModuleConfig{
			ModuleSourceKey:       testTemplatePath,
			ModuleVersionKey:      testTemplateVersion,
			"resource_group_name": envParams["resource_group_name"],
			"redis_cache_name":    resourceParams["redis_cache_name"],
			"sku":                 resourceParams["sku"],
		}

		moduleData := newModuleConfig(testTemplatePath, testTemplateVersion, envParams, resourceParams)

		// Assert that the module data contains the expected data.
		require.Equal(t, expectedModuleData, moduleData)
	})
	t.Run("Without templateVersion", func(t *testing.T) {
		expectedModuleData := TFModuleConfig{
			ModuleSourceKey:       testTemplatePath,
			"resource_group_name": envParams["resource_group_name"],
			"redis_cache_name":    resourceParams["redis_cache_name"],
			"sku":                 resourceParams["sku"],
		}

		moduleData := newModuleConfig(testTemplatePath, "", envParams, resourceParams)

		// Assert that the module data contains the expected data.
		require.Equal(t, expectedModuleData, moduleData)
	})

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
		Module: map[string]TFModuleConfig{
			testRecipeName: {
				ModuleSourceKey:       testTemplatePath,
				ModuleVersionKey:      testTemplateVersion,
				"resource_group_name": envParams["resource_group_name"],
				"redis_cache_name":    resourceParams["redis_cache_name"],
				"sku":                 resourceParams["sku"],
			},
		},
	}

	tfconfig := New(testRecipeName, testDir, &envRecipe, &resourceRecipe)
	err := tfconfig.Save(ctx)
	require.NoError(t, err)

	mProvider.EXPECT().BuildConfig(ctx, &envConfig).Times(1).Return(awsProviderConfig, nil)
	mProvider.EXPECT().BuildConfig(ctx, &envConfig).Times(1).Return(azureProviderConfig, nil)
	mProvider.EXPECT().BuildConfig(ctx, &envConfig).Times(1).Return(kubernetesProviderConfig, nil)

	err = tfconfig.AddProviders(ctx, []string{providers.AWSProviderName, providers.AzureProviderName, providers.KubernetesProviderName, "sql"}, supportedProviders, &envConfig)
	require.NoError(t, err)

	err = tfconfig.Save(ctx)
	require.NoError(t, err)

	// Validate that the config file exists and read the updated data.
	tfConfig, err := validateConfigIsGenerated(tfconfig.ConfigFilePath())
	require.NoError(t, err)
	// Assert that the TerraformConfig contains the expected data.
	require.Equal(t, expectedTFConfig, tfConfig)
}

func TestAddProviders_InvalidScope_Error(t *testing.T) {
	ctx := testcontext.New(t)
	// Create a temporary test directory.
	testDir := t.TempDir()
	mProvider, supportedProviders := setup(t)
	envRecipe, resourceRecipe := getTestInputs()
	envConfig := recipes.Configuration{
		Providers: datamodel.Providers{
			AWS: datamodel.ProvidersAWS{
				Scope: "invalid",
			},
		},
	}

	expectedTFConfig := TerraformConfig{
		Module: map[string]TFModuleConfig{
			testRecipeName: {
				ModuleSourceKey:       testTemplatePath,
				ModuleVersionKey:      testTemplateVersion,
				"resource_group_name": envParams["resource_group_name"],
				"redis_cache_name":    resourceParams["redis_cache_name"],
				"sku":                 resourceParams["sku"],
			},
		},
	}

	tfconfig := New(testRecipeName, testDir, &envRecipe, &resourceRecipe)
	err := tfconfig.Save(ctx)
	require.NoError(t, err)

	mProvider.EXPECT().BuildConfig(ctx, &envConfig).Times(1).Return(nil, errors.New("Invalid AWS provider scope"))

	err = tfconfig.AddProviders(ctx, []string{providers.AWSProviderName}, supportedProviders, &envConfig)
	require.Error(t, err)

	// Validate that the config file still exists and was not updated.
	tfConfig, err := validateConfigIsGenerated(tfconfig.ConfigFilePath())
	require.NoError(t, err)
	require.Equal(t, expectedTFConfig, tfConfig)
}

func TestAddProviders_EmptyProviderConfigurations_Success(t *testing.T) {
	ctx := testcontext.New(t)
	// Create a temporary test directory.
	testDir := t.TempDir()

	mProvider, supportedProviders := setup(t)
	envRecipe, resourceRecipe := getTestInputs()
	envConfig := recipes.Configuration{}

	// Expected config shouldn't contain any provider config
	expectedTFConfig := TerraformConfig{
		Module: map[string]TFModuleConfig{
			testRecipeName: {
				ModuleSourceKey:       testTemplatePath,
				ModuleVersionKey:      testTemplateVersion,
				"resource_group_name": envParams["resource_group_name"],
				"redis_cache_name":    resourceParams["redis_cache_name"],
				"sku":                 resourceParams["sku"],
			},
		},
	}

	tfconfig := New(testRecipeName, testDir, &envRecipe, &resourceRecipe)
	err := tfconfig.Save(ctx)
	require.NoError(t, err)

	// Expect build config function call for AWS provider with empty output since envConfig has empty AWS scope
	mProvider.EXPECT().BuildConfig(ctx, &envConfig).Times(1).Return(map[string]any{}, nil)

	err = tfconfig.AddProviders(ctx, []string{providers.AWSProviderName}, supportedProviders, &envConfig)
	require.NoError(t, err)

	err = tfconfig.Save(ctx)
	require.NoError(t, err)

	// Validate that the config file exists and read the updated data.
	tfConfig, err := validateConfigIsGenerated(tfconfig.ConfigFilePath())
	require.NoError(t, err)
	// Assert that the TerraformConfig contains the expected data.
	require.Equal(t, expectedTFConfig, tfConfig)
}

// Empty AWS scope should return empty AWS provider config
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

	// Expected config shouldn't contain any provider config
	expectedTFConfig := TerraformConfig{
		Module: map[string]TFModuleConfig{
			testRecipeName: {
				ModuleSourceKey:       testTemplatePath,
				ModuleVersionKey:      testTemplateVersion,
				"resource_group_name": envParams["resource_group_name"],
				"redis_cache_name":    resourceParams["redis_cache_name"],
				"sku":                 resourceParams["sku"],
			},
		},
	}

	tfconfig := New(testRecipeName, testDir, &envRecipe, &resourceRecipe)
	err := tfconfig.Save(ctx)
	require.NoError(t, err)

	mProvider.EXPECT().BuildConfig(ctx, &envConfig).Times(1).Return(nil, nil)

	err = tfconfig.AddProviders(ctx, []string{providers.AWSProviderName}, supportedProviders, &envConfig)
	require.NoError(t, err)
	err = tfconfig.Save(ctx)
	require.NoError(t, err)

	// Validate that the config file exists and read the updated data.
	tfConfig, err := validateConfigIsGenerated(tfconfig.ConfigFilePath())
	require.NoError(t, err)

	// Assert that the TerraformConfig contains the expected data.
	require.Equal(t, expectedTFConfig, tfConfig)
}

func TestAddProviders_MissingAzureProvider(t *testing.T) {
	ctx := testcontext.New(t)
	// Create a temporary test directory.
	testDir := t.TempDir()
	mProvider, supportedProviders := setup(t)
	envRecipe, resourceRecipe := getTestInputs()
	envConfig := recipes.Configuration{}

	azureProviderConfig := map[string]any{
		"features": map[string]any{},
	}
	// Expected config shouldn't contain Azure subscription id in the provider config
	expectedTFConfig := TerraformConfig{
		Provider: map[string]any{
			providers.AzureProviderName: azureProviderConfig,
		},
		Module: map[string]TFModuleConfig{
			testRecipeName: {
				ModuleSourceKey:       testTemplatePath,
				ModuleVersionKey:      testTemplateVersion,
				"resource_group_name": envParams["resource_group_name"],
				"redis_cache_name":    resourceParams["redis_cache_name"],
				"sku":                 resourceParams["sku"],
			},
		},
	}

	tfconfig := New(testRecipeName, testDir, &envRecipe, &resourceRecipe)
	err := tfconfig.Save(ctx)
	require.NoError(t, err)

	mProvider.EXPECT().BuildConfig(ctx, &envConfig).Times(1).Return(azureProviderConfig, nil)

	err = tfconfig.AddProviders(ctx, []string{providers.AzureProviderName}, supportedProviders, &envConfig)
	require.NoError(t, err)
	err = tfconfig.Save(ctx)
	require.NoError(t, err)

	// Validate that the config file exists and read the updated data.
	tfConfig, err := validateConfigIsGenerated(tfconfig.ConfigFilePath())
	require.NoError(t, err)

	// Assert that the TerraformConfig contains the expected data.
	require.Equal(t, expectedTFConfig, tfConfig)
}

func TestAddProviders_WriteConfigFileError(t *testing.T) {
	ctx := testcontext.New(t)
	mProvider, supportedProviders := setup(t)
	// Create a temporary test directory.
	testDir := t.TempDir()

	envRecipe, resourceRecipe := getTestInputs()
	tfconfig := New(testRecipeName, testDir, &envRecipe, &resourceRecipe)

	// Create a test configuration file.
	err := os.WriteFile(tfconfig.ConfigFilePath(), []byte(`{"module":{}}`), 0400)
	require.NoError(t, err)

	kubernetesProviderConfig := map[string]any{
		"config_path": clientcmd.RecommendedHomeFile,
	}
	mProvider.EXPECT().BuildConfig(ctx, nil).Times(1).Return(kubernetesProviderConfig, nil)

	// Call AddProviders with the test configuration file and required providers.
	err = tfconfig.AddProviders(ctx, []string{providers.KubernetesProviderName}, supportedProviders, nil)
	require.NoError(t, err)

	// Assert that AddProviders returns an error.
	err = tfconfig.Save(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "permission denied")
}
