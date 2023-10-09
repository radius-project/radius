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
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/recipecontext"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/backends"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/providers"
	"github.com/radius-project/radius/test/testcontext"
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
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.datastores/mongodatabases/mongo0",
				Name: "mongo0",
			},
			Type: "applications.datastores/mongodatabases",
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

func Test_NewConfig(t *testing.T) {
	configTests := []struct {
		desc               string
		moduleName         string
		envdef             *recipes.EnvironmentDefinition
		metadata           *recipes.ResourceMetadata
		expectedConfigFile string
	}{
		{
			desc:       "all non empty input params",
			moduleName: testRecipeName,
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
			expectedConfigFile: "testdata/module.tf.json",
		},
		{
			desc:       "empty recipe parameters",
			moduleName: testRecipeName,
			envdef: &recipes.EnvironmentDefinition{
				Name:            testRecipeName,
				TemplatePath:    testTemplatePath,
				TemplateVersion: testTemplateVersion,
			},
			metadata: &recipes.ResourceMetadata{
				Name: testRecipeName,
			},
			expectedConfigFile: "testdata/module-emptyparams.tf.json",
		},
		{
			desc:       "empty resource metadata",
			moduleName: testRecipeName,
			envdef: &recipes.EnvironmentDefinition{
				Name:            testRecipeName,
				TemplatePath:    testTemplatePath,
				TemplateVersion: testTemplateVersion,
				Parameters:      envParams,
			},
			metadata:           &recipes.ResourceMetadata{},
			expectedConfigFile: "testdata/module-emptyresourceparam.tf.json",
		},
		{
			desc:       "empty template version",
			moduleName: testRecipeName,
			envdef: &recipes.EnvironmentDefinition{
				Name:         testRecipeName,
				TemplatePath: testTemplatePath,
				Parameters:   envParams,
			},
			metadata: &recipes.ResourceMetadata{
				Name:       testRecipeName,
				Parameters: resourceParams,
			},
			expectedConfigFile: "testdata/module-emptytemplateversion.tf.json",
		},
	}

	for _, tc := range configTests {
		t.Run(tc.desc, func(t *testing.T) {
			workingDir := t.TempDir()

			tfconfig := New(testRecipeName, tc.envdef, tc.metadata)

			// validate generated config
			err := tfconfig.Save(testcontext.New(t), workingDir)
			require.NoError(t, err)
			actualConfig, err := os.ReadFile(getMainConfigFilePath(workingDir))
			require.NoError(t, err)
			expectedConfig, err := os.ReadFile(tc.expectedConfigFile)
			require.NoError(t, err)
			require.Equal(t, string(expectedConfig), string(actualConfig))
		})
	}
}

func Test_AddRecipeContext(t *testing.T) {
	configTests := []struct {
		desc               string
		moduleName         string
		envdef             *recipes.EnvironmentDefinition
		metadata           *recipes.ResourceMetadata
		recipeContext      *recipecontext.Context
		expectedConfigFile string
		err                string
	}{
		{
			desc:       "non empty recipe context and input recipe paramters",
			moduleName: testRecipeName,
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
			expectedConfigFile: "testdata/recipecontext.tf.json",
		},
		{
			desc:       "non empty recipe context, empty input recipe parameters",
			moduleName: testRecipeName,
			envdef: &recipes.EnvironmentDefinition{
				Name:            testRecipeName,
				TemplatePath:    testTemplatePath,
				TemplateVersion: testTemplateVersion,
			},
			metadata: &recipes.ResourceMetadata{
				Name: testRecipeName,
			},
			recipeContext:      getTestRecipeContext(),
			expectedConfigFile: "testdata/recipecontext-emptyrecipeparams.tf.json",
		},
		{
			desc:       "empty recipe context",
			moduleName: testRecipeName,
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
			expectedConfigFile: "testdata/module.tf.json",
		},
		{
			desc:       "invalid module name",
			moduleName: "invalid",
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
			expectedConfigFile: "testdata/module.tf.json",
			err:                "module \"invalid\" not found in the initialized terraform config",
		},
	}

	for _, tc := range configTests {
		t.Run(tc.desc, func(t *testing.T) {
			ctx := testcontext.New(t)
			workingDir := t.TempDir()

			tfconfig := New(testRecipeName, tc.envdef, tc.metadata)

			err := tfconfig.AddRecipeContext(ctx, tc.moduleName, tc.recipeContext)
			if tc.err == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Equal(t, tc.err, err.Error())
			}

			err = tfconfig.Save(ctx, workingDir)
			require.NoError(t, err)

			// validate generated config
			actualConfig, err := os.ReadFile(getMainConfigFilePath(workingDir))
			require.NoError(t, err)
			expectedConfig, err := os.ReadFile(tc.expectedConfigFile)
			require.NoError(t, err)
			require.Equal(t, string(expectedConfig), string(actualConfig))
		})
	}
}

func Test_AddProviders(t *testing.T) {
	mProvider, supportedProviders, mBackend := setup(t)
	envRecipe, resourceRecipe := getTestInputs()
	expectedBackend := map[string]any{
		"kubernetes": map[string]any{
			"config_path":   "/home/radius/.kube/config",
			"secret_suffix": "test-secret-suffix",
			"namespace":     "radius-system",
		},
	}
	configTests := []struct {
		desc               string
		envConfig          recipes.Configuration
		requiredProviders  []string
		expectedProviders  []map[string]any
		expectedConfigFile string
		Err                error
	}{
		{
			desc: "valid all supported providers",
			expectedProviders: []map[string]any{
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
			Err: nil,
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

			expectedConfigFile: "testdata/providers-valid.tf.json",
		},
		{
			desc:              "invalid aws scope",
			expectedProviders: nil,
			Err:               errors.New("Invalid AWS provider scope"),
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
			desc: "empty aws provider config",
			expectedProviders: []map[string]any{
				{},
			},
			Err:       nil,
			envConfig: recipes.Configuration{},
			requiredProviders: []string{
				providers.AWSProviderName,
			},
			expectedConfigFile: "testdata/providers-empty.tf.json",
		},
		{
			desc: "empty aws scope",
			expectedProviders: []map[string]any{
				nil,
			},
			Err: nil,
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
			expectedConfigFile: "testdata/providers-empty.tf.json",
		},
		{
			desc: "empty azure provider config",
			expectedProviders: []map[string]any{
				{
					"features": map[string]any{},
				},
			},
			Err:       nil,
			envConfig: recipes.Configuration{},
			requiredProviders: []string{
				providers.AzureProviderName,
			},
			expectedConfigFile: "testdata/providers-emptyazureconfig.tf.json",
		},
	}

	for _, tc := range configTests {
		t.Run(tc.desc, func(t *testing.T) {
			ctx := testcontext.New(t)
			workingDir := t.TempDir()

			tfconfig := New(testRecipeName, &envRecipe, &resourceRecipe)
			for _, p := range tc.expectedProviders {
				mProvider.EXPECT().BuildConfig(ctx, &tc.envConfig).Times(1).Return(p, nil)
			}
			if tc.Err != nil {
				mProvider.EXPECT().BuildConfig(ctx, &tc.envConfig).Times(1).Return(nil, tc.Err)
			}
			err := tfconfig.AddProviders(ctx, tc.requiredProviders, supportedProviders, &tc.envConfig)
			if tc.Err != nil {
				require.ErrorContains(t, err, tc.Err.Error())
				return
			}
			require.NoError(t, err)
			mBackend.EXPECT().BuildBackend(&resourceRecipe).AnyTimes().Return(expectedBackend, nil)
			_, err = tfconfig.AddTerraformBackend(&resourceRecipe, mBackend)
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

func Test_AddOutputs(t *testing.T) {
	envRecipe, resourceRecipe := getTestInputs()
	tests := []struct {
		desc                 string
		moduleName           string
		expectedOutputConfig map[string]any
		expectedConfigFile   string
		expectedErr          bool
	}{
		{
			desc:       "valid output",
			moduleName: testRecipeName,
			expectedOutputConfig: map[string]any{
				"result": map[string]any{
					"value":     "${module.redis-azure.result}",
					"sensitive": true,
				},
			},
			expectedConfigFile: "testdata/outputs.tf.json",
			expectedErr:        false,
		},
		{
			desc:               "empty module name",
			moduleName:         "",
			expectedConfigFile: "testdata/module.tf.json",
			expectedErr:        true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			tfconfig := New(testRecipeName, &envRecipe, &resourceRecipe)

			err := tfconfig.AddOutputs(tc.moduleName)
			if tc.expectedErr {
				require.Error(t, err)
				require.Nil(t, tfconfig.Output)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedOutputConfig, tfconfig.Output)
			}

			workingDir := t.TempDir()
			err = tfconfig.Save(testcontext.New(t), workingDir)
			require.NoError(t, err)

			// Assert generated config file matches expected config in JSON format.
			actualConfig, err := os.ReadFile(getMainConfigFilePath(workingDir))
			require.NoError(t, err)
			expectedConfig, err := os.ReadFile(tc.expectedConfigFile)
			require.NoError(t, err)
			require.Equal(t, string(expectedConfig), string(actualConfig))
		})
	}
}

func Test_Save_overwrite(t *testing.T) {
	ctx := testcontext.New(t)
	testDir := t.TempDir()
	envRecipe, resourceRecipe := getTestInputs()
	tfconfig := New(testRecipeName, &envRecipe, &resourceRecipe)

	err := tfconfig.Save(ctx, testDir)
	require.NoError(t, err)

	err = tfconfig.Save(ctx, testDir)
	require.NoError(t, err)
}

func Test_Save_ConfigFileReadOnly(t *testing.T) {
	testDir := t.TempDir()
	envRecipe, resourceRecipe := getTestInputs()
	tfconfig := New(testRecipeName, &envRecipe, &resourceRecipe)

	// Create a test configuration file with read only permission.
	err := os.WriteFile(getMainConfigFilePath(testDir), []byte(`{"module":{}}`), 0400)
	require.NoError(t, err)

	// Assert that Save returns an error.
	err = tfconfig.Save(testcontext.New(t), testDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "permission denied")
}

func Test_Save_InvalidWorkingDir(t *testing.T) {
	testDir := filepath.Join("invalid", uuid.New().String())
	envRecipe, resourceRecipe := getTestInputs()

	tfconfig := New(testRecipeName, &envRecipe, &resourceRecipe)

	err := tfconfig.Save(testcontext.New(t), testDir)
	require.Error(t, err)
	require.Equal(t, fmt.Sprintf("error creating file: open %s/main.tf.json: no such file or directory", testDir), err.Error())
}
