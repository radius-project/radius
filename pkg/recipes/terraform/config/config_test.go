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
	"github.com/project-radius/radius/pkg/recipes/terraform/config/backends"
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

func setup(t *testing.T) (providers.MockProvider, map[string]providers.Provider, *backends.MockBackend) {
	ctrl := gomock.NewController(t)
	mProvider := providers.NewMockProvider(ctrl)
	mBackend := backends.NewMockBackend(ctrl)
	providers := map[string]providers.Provider{
		providers.AWSProviderName:        mProvider,
		providers.AzureProviderName:      mProvider,
		providers.KubernetesProviderName: mProvider,
	}

	return *mProvider, providers, mBackend
}

func getTestRecipeContext() *recipecontext.Context {
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
		Name:          testRecipeName,
		Parameters:    resourceParams,
		EnvironmentID: "/planes/radius/local/resourceGroups/test-group/providers/Applications.Environments/testEnv/env",
		ApplicationID: "/planes/radius/local/resourceGroups/test-group/providers/Applications.Applications/testApp/app",
		ResourceID:    "/planes/radius/local/resourceGroups/test-group/providers/Applications.Datastores/redisCaches/redis",
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
			name: "recipe context, env, and resource metadata params are given",
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
			recipeContext:      getTestRecipeContext(),
			expectedConfigFile: "testdata/main-all.tf.json",
		},
		{
			name: "only recipe context is given without env and resource metadata params",
			envdef: &recipes.EnvironmentDefinition{
				Name:            testRecipeName,
				TemplatePath:    testTemplatePath,
				TemplateVersion: testTemplateVersion,
			},
			metadata: &recipes.ResourceMetadata{
				Name: testRecipeName,
			},
			recipeContext:      getTestRecipeContext(),
			expectedConfigFile: "testdata/main-noparams.tf.json",
		},
		{
			name: "recipe context and env params are given",
			envdef: &recipes.EnvironmentDefinition{
				Name:            testRecipeName,
				TemplatePath:    testTemplatePath,
				TemplateVersion: testTemplateVersion,
				Parameters:      envParams,
			},
			metadata: &recipes.ResourceMetadata{
				Name: testRecipeName,
			},
			recipeContext:      getTestRecipeContext(),
			expectedConfigFile: "testdata/main-noresourceparam.tf.json",
		},
		{
			name: "env and resource metadata params are given without recipe context",
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
			expectedConfigFile: "testdata/main-nocontext.tf.json",
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
			expectedConfigFile: "testdata/main-notplver.tf.json",
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
			tfconfig := New(testRecipeName, tc.envdef, tc.metadata)
			if tc.recipeContext != nil {
				err := tfconfig.AddRecipeContext(ctx, testRecipeName, tc.recipeContext)
				require.NoError(t, err)
			}
			err := tfconfig.Save(ctx, tc.configPath)
			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				return
			}

			require.NoError(t, err)

			// assert
			actualConfig, err := os.ReadFile(getMainConfigFilePath(tc.configPath))
			require.NoError(t, err)
			expectedConfig, err := os.ReadFile(tc.expectedConfigFile)
			require.NoError(t, err)
			require.Equal(t, string(expectedConfig), string(actualConfig))
		})
	}
}

func TestAddProviders(t *testing.T) {
	mProvider, supportedProviders, _ := setup(t)
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

			expectedConfigFile: "testdata/main-provider-valid.tf.json",
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
			expectedConfigFile: "testdata/main-provider-empty.tf.json",
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
			expectedConfigFile: "testdata/main-provider-empty.tf.json",
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
			expectedConfigFile: "testdata/main-provider-missingazure.tf.json",
		},
	}

	for _, tc := range configTests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := testcontext.New(t)
			workingDir := t.TempDir()

			tfconfig := New(testRecipeName, &envRecipe, &resourceRecipe)
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
			err = tfconfig.Save(ctx, workingDir)
			require.NoError(t, err)

			// assert
			actualConfig, err := os.ReadFile(getMainConfigFilePath(workingDir))
			require.NoError(t, err)
			expectedConfig, err := os.ReadFile(tc.expectedConfigFile)
			require.NoError(t, err)
			require.Equal(t, string(expectedConfig), string(actualConfig))
		})
	}
}

func TestAddBackend(t *testing.T) {
	_, _, mBackend := setup(t)
	envRecipe, resourceRecipe := getTestInputs()
	configTests := []struct {
		name               string
		envConfig          recipes.Configuration
		expectedBackend    map[string]any
		expectedConfigFile string
	}{
		{
			name: "valid-backend-kubernetes",
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
			expectedBackend: map[string]any{
				"kubernetes": map[string]any{
					"config_path":   "/home/radius/.kube/config",
					"secret_suffix": "test-secret-suffix",
					"namespace":     "radius-system",
				},
			},
			expectedConfigFile: "testdata/main-backend-kubernetes-valid.tf.json",
		},
	}
	for _, tc := range configTests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := testcontext.New(t)
			workingDir := t.TempDir()
			tfconfig := New(testRecipeName, &envRecipe, &resourceRecipe)
			mBackend.EXPECT().BuildBackend(&resourceRecipe).AnyTimes().Return(tc.expectedBackend, nil)
			_, err := tfconfig.AddBackend(&resourceRecipe, mBackend)
			require.NoError(t, err)
			err = tfconfig.Save(ctx, workingDir)
			require.NoError(t, err)

			// assert
			actualConfig, err := os.ReadFile(getMainConfigFilePath(workingDir))
			require.NoError(t, err)
			expectedConfig, err := os.ReadFile(tc.expectedConfigFile)
			require.NoError(t, err)
			require.Equal(t, string(expectedConfig), string(actualConfig))
		})
	}
}

func TestSave_overwrite(t *testing.T) {
	ctx := testcontext.New(t)
	testDir := t.TempDir()
	envRecipe, resourceRecipe := getTestInputs()
	tfconfig := New(testRecipeName, &envRecipe, &resourceRecipe)

	err := tfconfig.Save(ctx, testDir)
	require.NoError(t, err)

	err = tfconfig.Save(ctx, testDir)
	require.NoError(t, err)
}

func TestSave_Failure(t *testing.T) {
	ctx := testcontext.New(t)
	testDir := t.TempDir()
	envRecipe, resourceRecipe := getTestInputs()
	tfconfig := New(testRecipeName, &envRecipe, &resourceRecipe)

	// Create a test configuration file.
	err := os.WriteFile(getMainConfigFilePath(testDir), []byte(`{"module":{}}`), 0400)
	require.NoError(t, err)

	// Assert that AddProviders returns an error.
	err = tfconfig.Save(ctx, testDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "permission denied")
}
