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
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

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
			name: "context, environment definition, and metadata params",
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
			expectedConfigFile: "testdata/main.tf-all.json",
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
			name: "without template version",
			envdef: &recipes.EnvironmentDefinition{
				Name:         testRecipeName,
				TemplatePath: testTemplatePath,
				Parameters:   envParams,
			},
			metadata: &recipes.ResourceMetadata{
				Name:       testRecipeName,
				Parameters: resourceParams,
			},
			recipeContext:      nil,
			expectedConfigFile: "testdata/main.tf-notplver.json",
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

func TestAddProviders(t *testing.T) {
	mProvider, supportedProviders := setup(t)
	envRecipe, resourceRecipe := getTestInputs()

	configTests := []struct {
		name               string
		modProviders       []map[string]any
		modProviderErr     error
		envConfig          recipes.Configuration
		requiredProviders  []string
		expectedConfigFile string
	}{
		{
			name: "valid-aws-azure-k8s-providers",
			modProviders: []map[string]any{
				{
					"region": "test-region",
				},
				{
					"subscription_id": "test-sub",
					"features":        map[string]any{},
				},
				{
					"config_path": "/home/radius/.kube/config",
				},
			},
			modProviderErr: nil,
			envConfig: recipes.Configuration{
				Providers: datamodel.Providers{
					AWS: datamodel.ProvidersAWS{
						Scope: "/planes/aws/aws/accounts/0000/regions/test-region",
					},
					Azure: datamodel.ProvidersAzure{
						Scope: "/subscriptions/test-sub/resourceGroups/test-rg",
					},
				},
			},
			requiredProviders: []string{
				providers.AWSProviderName,
				providers.AzureProviderName,
				providers.KubernetesProviderName,
				"sql",
			},
			expectedConfigFile: "testdata/main.tf-provider-valid.json",
		},
		{
			name:           "invalid scope",
			modProviders:   nil,
			modProviderErr: errors.New("Invalid AWS provider scope"),
			envConfig: recipes.Configuration{
				Providers: datamodel.Providers{
					AWS: datamodel.ProvidersAWS{
						Scope: "invalid",
					},
				},
			},
			requiredProviders: []string{
				providers.AWSProviderName,
			},
		},
		{
			name: "empty provider",
			modProviders: []map[string]any{
				{},
			},
			modProviderErr: nil,
			envConfig:      recipes.Configuration{},
			requiredProviders: []string{
				providers.AWSProviderName,
			},
			expectedConfigFile: "testdata/main.tf-provider-empty.json",
		},
		{
			name: "empty aws scope",
			modProviders: []map[string]any{
				nil,
			},
			modProviderErr: nil,
			envConfig: recipes.Configuration{
				Providers: datamodel.Providers{
					AWS: datamodel.ProvidersAWS{
						Scope: "",
					},
					Azure: datamodel.ProvidersAzure{
						Scope: "/subscriptions/test-sub/resourceGroups/test-rg",
					},
				},
			},
			requiredProviders: []string{
				providers.AWSProviderName,
			},
			expectedConfigFile: "testdata/main.tf-provider-empty.json",
		},
		{
			name: "missing azure provider",
			modProviders: []map[string]any{
				{
					"features": map[string]any{},
				},
			},
			modProviderErr: nil,
			envConfig:      recipes.Configuration{},
			requiredProviders: []string{
				providers.AzureProviderName,
			},
			expectedConfigFile: "testdata/main.tf-provider-missingazure.json",
		},
	}

	for _, tc := range configTests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := testcontext.New(t)
			workingDir := t.TempDir()

			tfconfig := New(testRecipeName, workingDir, &envRecipe, &resourceRecipe)

			for _, p := range tc.modProviders {
				mProvider.EXPECT().BuildConfig(ctx, &tc.envConfig).Times(1).Return(p, nil)
			}

			if tc.modProviderErr != nil {
				mProvider.EXPECT().BuildConfig(ctx, &tc.envConfig).Times(1).Return(nil, tc.modProviderErr)
			}

			err := tfconfig.AddProviders(ctx, tc.requiredProviders, supportedProviders, &tc.envConfig)
			if tc.modProviderErr != nil {
				require.ErrorContains(t, err, tc.modProviderErr.Error())
				return
			}

			require.NoError(t, err)

			err = tfconfig.Save(ctx)
			require.NoError(t, err)

			t.Log(tfconfig.ConfigFilePath())

			// assert
			actualConfig, err := os.ReadFile(tfconfig.ConfigFilePath())
			require.NoError(t, err)
			expectedConfig, err := os.ReadFile(tc.expectedConfigFile)
			require.NoError(t, err)
			require.Equal(t, string(expectedConfig), string(actualConfig))
		})
	}
}

func TestSave_overwrite(t *testing.T) {
	ctx := testcontext.New(t)
	envRecipe, resourceRecipe := getTestInputs()
	tfconfig := New(testRecipeName, t.TempDir(), &envRecipe, &resourceRecipe)

	err := tfconfig.Save(ctx)
	require.NoError(t, err)

	err = tfconfig.Save(ctx)
	require.NoError(t, err)
}

func TestSave_Failure(t *testing.T) {
	ctx := testcontext.New(t)
	envRecipe, resourceRecipe := getTestInputs()
	tfconfig := New(testRecipeName, t.TempDir(), &envRecipe, &resourceRecipe)

	// Create a test configuration file.
	err := os.WriteFile(tfconfig.ConfigFilePath(), []byte(`{"module":{}}`), 0400)
	require.NoError(t, err)

	// Assert that AddProviders returns an error.
	err = tfconfig.Save(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "permission denied")
}
