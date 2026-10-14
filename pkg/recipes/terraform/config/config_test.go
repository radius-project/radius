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
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"uuid"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/recipecontext"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/backends"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/providers"
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

			tfconfig, err := New(t.Context(), testRecipeName, tc.envdef, tc.metadata)
			require.NoError(t, err)

			// validate generated config
			err = tfconfig.Save(t.Context(), workingDir)
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
			desc:       "non empty recipe context and input recipe parameters",
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
			ctx := t.Context()
			workingDir := t.TempDir()

			tfconfig, err := New(t.Context(), testRecipeName, tc.envdef, tc.metadata)
			require.NoError(t, err)
			err = tfconfig.AddRecipeContext(ctx, tc.moduleName, tc.recipeContext)
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
	mProvider, ucpConfiguredProviders, mBackend := setup(t)
	envRecipe, resourceRecipe := getTestInputs()
	expectedBackend := map[string]any{
		"kubernetes": map[string]any{
			"config_path":   "/home/radius/.kube/config",
			"secret_suffix": "test-secret-suffix",
			"namespace":     "radius-system",
		},
	}

	configTests := []struct {
		desc                           string
		envConfig                      recipes.Configuration
		requiredProviders              map[string]*RequiredProviderInfo
		expectedUCPConfiguredProviders []map[string]any
		useUCPProviderConfig           bool
		expectedConfigFile             string
		Err                            error
	}{
		{
			desc: "valid all supported providers",
			expectedUCPConfiguredProviders: []map[string]any{
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
			requiredProviders: map[string]*RequiredProviderInfo{
				providers.AWSProviderName:        {},
				providers.AzureProviderName:      {},
				providers.KubernetesProviderName: {},
				"sql":                            {},
			},
			useUCPProviderConfig: true,
			expectedConfigFile:   "testdata/providers-valid.tf.json",
		},
		{
			desc:                           "invalid aws scope",
			expectedUCPConfiguredProviders: nil,
			Err:                            errors.New("Invalid AWS provider scope"),
			envConfig: recipes.Configuration{
				Providers: datamodel.Providers{
					AWS: datamodel.ProvidersAWS{
						Scope: "invalid",
					},
				},
			},
			requiredProviders: map[string]*RequiredProviderInfo{
				providers.AWSProviderName: {
					Source:  "hashicorp/aws",
					Version: ">= 3.0",
				},
			},
			useUCPProviderConfig: true,
		},
		{
			desc: "empty aws provider config with required provider",
			expectedUCPConfiguredProviders: []map[string]any{
				{},
			},
			Err:       nil,
			envConfig: recipes.Configuration{},
			requiredProviders: map[string]*RequiredProviderInfo{
				providers.AWSProviderName: {},
			},
			useUCPProviderConfig: true,
			expectedConfigFile:   "testdata/providers-emptywithrequiredprovider.tf.json",
		},
		{
			desc: "empty aws scope",
			expectedUCPConfiguredProviders: []map[string]any{
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
			requiredProviders: map[string]*RequiredProviderInfo{
				providers.AWSProviderName: {},
			},
			useUCPProviderConfig: true,
			expectedConfigFile:   "testdata/providers-emptywithrequiredprovider.tf.json",
		},
		{
			desc: "empty azure provider config",
			expectedUCPConfiguredProviders: []map[string]any{
				{
					"features": map[string]any{},
				},
			},
			Err:       nil,
			envConfig: recipes.Configuration{},
			requiredProviders: map[string]*RequiredProviderInfo{
				providers.AzureProviderName: {
					Source:  "hashicorp/azurerm",
					Version: "~> 2.0",
				},
			},
			useUCPProviderConfig: true,
			expectedConfigFile:   "testdata/providers-emptyazureconfig.tf.json",
		},
		{
			desc:                           "valid recipe providers in env config",
			expectedUCPConfiguredProviders: nil,
			Err:                            nil,
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Providers: map[string][]datamodel.ProviderConfigProperties{
							"azurerm": {
								{
									AdditionalProperties: map[string]any{
										"subscriptionid": 1234,
										"tenant_id":      "745fg88bf-86f1-41af-43ut",
									},
								},
								{
									AdditionalProperties: map[string]any{
										"alias":          "az-paymentservice",
										"subscriptionid": 45678,
										"tenant_id":      "gfhf45345-5d73-gh34-wh84",
									},
								},
							},
						},
					},
				},
			},
			requiredProviders:  nil,
			expectedConfigFile: "testdata/providers-envrecipeproviders.tf.json",
		},
		{
			desc: "recipe provider config overridding ucp provider configs",
			expectedUCPConfiguredProviders: []map[string]any{
				{
					"region": "test-region",
				},
				{
					"config_path": "/home/radius/.kube/UCPconfig",
				},
			},
			Err:                  nil,
			useUCPProviderConfig: false,
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Providers: map[string][]datamodel.ProviderConfigProperties{
							"aws": {
								{
									AdditionalProperties: map[string]any{
										"region": "us-west-2",
									},
								},
							},
							"kubernetes": {
								{
									AdditionalProperties: map[string]any{
										"alias":       "k8s_first",
										"config_path": "/home/radius/.kube/configPath1",
									},
								},
								{
									AdditionalProperties: map[string]any{
										"alias":       "k8s_second",
										"config_path": "/home/radius/.kube/configPath2",
									},
								},
							},
						},
					},
				},
			},
			requiredProviders: map[string]*RequiredProviderInfo{
				providers.AWSProviderName: {
					Source:  "hashicorp/aws",
					Version: ">= 3.0",
				},
				providers.KubernetesProviderName: {
					Source:               "hashicorp/kubernetes",
					Version:              ">= 2.0",
					ConfigurationAliases: []string{"kubernetes.k8s_first", "kubernetes.k8s_second"},
				},
			},
			expectedConfigFile: "testdata/providers-overrideucpproviderconfig.tf.json",
		},
		{
			desc:                           "recipe providers in env config setup but nil",
			expectedUCPConfiguredProviders: nil,
			Err:                            nil,
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Providers: map[string][]datamodel.ProviderConfigProperties{
							"azurerm": {
								{
									AdditionalProperties: nil,
								},
								{
									AdditionalProperties: map[string]any{
										"alias":          "az-paymentservice",
										"subscriptionid": 45678,
										"tenant_id":      "gfhf45345-5d73-gh34-wh84",
									},
								},
							},
						},
					},
				},
			},
			requiredProviders:  nil,
			expectedConfigFile: "testdata/providers-envrecipedefaultconfig.tf.json",
		},
		{
			desc:                           "recipe providers not populated",
			expectedUCPConfiguredProviders: nil,
			Err:                            nil,
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{},
				},
			},
			requiredProviders:  nil,
			expectedConfigFile: "testdata/providers-empty.tf.json",
		},
		{
			desc:                           "recipe providers and tfconfigproperties not populated",
			expectedUCPConfiguredProviders: nil,
			Err:                            nil,
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{},
			},
			requiredProviders:  nil,
			expectedConfigFile: "testdata/providers-empty.tf.json",
		},
		{
			desc:                           "envConfig set to empty recipe config",
			expectedUCPConfiguredProviders: nil,
			Err:                            nil,
			envConfig:                      recipes.Configuration{},
			requiredProviders:              nil,
			expectedConfigFile:             "testdata/providers-empty.tf.json",
		},
		{
			desc:                           "envConfig not populated",
			expectedUCPConfiguredProviders: nil,
			Err:                            nil,
			requiredProviders:              nil,
			expectedConfigFile:             "testdata/providers-empty.tf.json",
		},
	}

	for _, tc := range configTests {
		t.Run(tc.desc, func(t *testing.T) {
			ctx := t.Context()
			workingDir := t.TempDir()

			tfconfig, err := New(ctx, testRecipeName, &envRecipe, &resourceRecipe)
			require.NoError(t, err)
			if tc.useUCPProviderConfig {
				for _, p := range tc.expectedUCPConfiguredProviders {
					mProvider.EXPECT().BuildConfig(ctx, &tc.envConfig).Times(1).Return(p, nil)
				}
			}
			if tc.Err != nil {
				mProvider.EXPECT().BuildConfig(ctx, &tc.envConfig).Times(1).Return(nil, tc.Err)
			}
			err = tfconfig.AddProviders(ctx, tc.requiredProviders, ucpConfiguredProviders, &tc.envConfig, nil)
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
			var actualConfig, expectedConfig map[string]any

			actualConfigBytes, err := os.ReadFile(getMainConfigFilePath(workingDir))
			require.NoError(t, err)
			err = json.Unmarshal(actualConfigBytes, &actualConfig)
			require.NoError(t, err)

			expectedConfigBytes, err := os.ReadFile(tc.expectedConfigFile)
			require.NoError(t, err)
			err = json.Unmarshal(expectedConfigBytes, &expectedConfig)
			require.NoError(t, err)

			// For tests with multiple UCPConfigured providers and 'useUCPProviderConfig' set to true, we validate test based on count of provider configurations and matching provider names (keys).
			if len(tc.expectedUCPConfiguredProviders) > 1 && tc.useUCPProviderConfig == true {

				// The AddProviders function accepts a map of UCPConfigured providers. As maps in Go do not guarantee iteration order, the sequence of providers in the resulting configuration can vary.
				// The mock call to BuildConfig() anticipates a specific provider configuration output in a fixed order.
				// Due to potential discrepancies in order, a deep comparison of the two maps is not feasible. Instead, we compare the count and match keys of provider configurations.
				expectedProviderMap := expectedConfig["provider"].(map[string]any)
				actualProviderMap := actualConfig["provider"].(map[string]any)

				require.Equal(t, len(expectedProviderMap), len(actualProviderMap))
				for k := range expectedProviderMap {
					require.Contains(t, actualProviderMap, k)
				}
			} else {
				// This performs a deep comparison of the two maps.
				require.Equal(t, expectedConfig, actualConfig)
			}
		})
	}
}

func Test_AddProviderAliasPlaceholders(t *testing.T) {
	envConfig := &recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				Providers: map[string][]datamodel.ProviderConfigProperties{
					"azurerm": {
						{AdditionalProperties: map[string]any{"subscription_id": "sub"}},
						{AdditionalProperties: map[string]any{"alias": "custom", "subscription_id": "sub"}},
						{AdditionalProperties: map[string]any{"alias": "custom", "subscription_id": "other"}},
						{AdditionalProperties: map[string]any{"alias": "unused", "subscription_id": "sub"}},
					},
					"postgresql": {
						{AdditionalProperties: map[string]any{"alias": "pgdb-test", "host": "localhost"}},
					},
					"kubectl": {
						{AdditionalProperties: map[string]any{"alias": "custom"}},
					},
				},
			},
		},
	}

	tests := []struct {
		desc                      string
		envConfig                 *recipes.Configuration
		requiredProviders         map[string]*RequiredProviderInfo
		expectedAdded             bool
		expectedProvider          map[string][]map[string]any
		expectedRequiredProviders map[string]*RequiredProviderInfo
		expectedProviders         map[string]string
	}{
		{
			desc: "nil env config",
			requiredProviders: map[string]*RequiredProviderInfo{
				"azurerm": {Source: "hashicorp/azurerm", ConfigurationAliases: []string{"azurerm.custom"}},
			},
		},
		{
			desc:      "no recipe providers",
			envConfig: &recipes.Configuration{},
			requiredProviders: map[string]*RequiredProviderInfo{
				"azurerm": {Source: "hashicorp/azurerm", ConfigurationAliases: []string{"azurerm.custom"}},
			},
		},
		{
			// A module that uses only the default provider must not get alias placeholders.
			desc:      "module does not declare configuration_aliases",
			envConfig: envConfig,
			requiredProviders: map[string]*RequiredProviderInfo{
				"azurerm": {Source: "hashicorp/azurerm"},
				"kubectl": {Source: "alekc/kubectl"},
			},
		},
		{
			desc:      "module declares an alias that is not in the environment",
			envConfig: envConfig,
			requiredProviders: map[string]*RequiredProviderInfo{
				"azurerm": {Source: "hashicorp/azurerm", ConfigurationAliases: []string{"azurerm.missing"}},
			},
		},
		{
			desc:      "module declares configuration_aliases",
			envConfig: envConfig,
			requiredProviders: map[string]*RequiredProviderInfo{
				"azurerm":    {Source: "hashicorp/azurerm", Version: ">= 3.0", ConfigurationAliases: []string{"azurerm.custom"}},
				"postgresql": {Source: "cyrilgdn/postgresql", ConfigurationAliases: []string{"postgresql.pgdb-test"}},
				"kubectl":    {Source: "alekc/kubectl"},
			},
			expectedAdded: true,
			expectedProvider: map[string][]map[string]any{
				"azurerm":    {{"alias": "custom"}},
				"postgresql": {{"alias": "pgdb-test"}},
			},
			expectedRequiredProviders: map[string]*RequiredProviderInfo{
				"azurerm":    {Source: "hashicorp/azurerm"},
				"postgresql": {Source: "cyrilgdn/postgresql"},
			},
			expectedProviders: map[string]string{
				"azurerm.custom":       "azurerm.custom",
				"postgresql.pgdb-test": "postgresql.pgdb-test",
			},
		},
		{
			desc:      "module declares configuration_aliases without a source",
			envConfig: envConfig,
			requiredProviders: map[string]*RequiredProviderInfo{
				"azurerm": {ConfigurationAliases: []string{"azurerm.custom"}},
			},
			expectedAdded:    true,
			expectedProvider: map[string][]map[string]any{"azurerm": {{"alias": "custom"}}},
			expectedProviders: map[string]string{
				"azurerm.custom": "azurerm.custom",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			envRecipe, resourceRecipe := getTestInputs()
			tfconfig, err := New(t.Context(), testRecipeName, &envRecipe, &resourceRecipe)
			require.NoError(t, err)

			added := tfconfig.AddProviderAliasPlaceholders(tc.envConfig, tc.requiredProviders)
			require.Equal(t, tc.expectedAdded, added)

			require.Equal(t, tc.expectedProvider, tfconfig.Provider)
			if tc.expectedRequiredProviders == nil {
				require.Nil(t, tfconfig.Terraform)
			} else {
				require.Equal(t, tc.expectedRequiredProviders, tfconfig.Terraform.RequiredProviders)

				// Terraform does not accept a null backend, so the saved configuration must not contain it.
				content, err := json.Marshal(tfconfig)
				require.NoError(t, err)
				require.NotContains(t, string(content), "backend")
			}

			moduleProviders, ok := tfconfig.Module[testRecipeName][moduleProvidersKey]
			if tc.expectedProviders == nil {
				require.False(t, ok)
				return
			}
			require.Equal(t, tc.expectedProviders, moduleProviders)
		})
	}
}

func Test_AddProviders_ReplacesAliasPlaceholders(t *testing.T) {
	aliasEnvConfig := &recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				Providers: map[string][]datamodel.ProviderConfigProperties{
					"azurerm": {
						{AdditionalProperties: map[string]any{"alias": "custom", "subscription_id": "sub"}},
					},
				},
			},
		},
	}

	tests := []struct {
		desc              string
		envConfig         *recipes.Configuration
		requiredProviders map[string]*RequiredProviderInfo
		expectedProvider  map[string][]map[string]any
		expectedProviders map[string]string
	}{
		{
			desc:      "module declares the alias",
			envConfig: aliasEnvConfig,
			requiredProviders: map[string]*RequiredProviderInfo{
				providers.AzureProviderName: {
					Source:               "hashicorp/azurerm",
					ConfigurationAliases: []string{"azurerm.custom"},
				},
			},
			expectedProvider: map[string][]map[string]any{
				"azurerm": {{"alias": "custom", "subscription_id": "sub"}},
			},
			expectedProviders: map[string]string{"azurerm.custom": "azurerm.custom"},
		},
		{
			desc:      "module does not declare the alias",
			envConfig: aliasEnvConfig,
			requiredProviders: map[string]*RequiredProviderInfo{
				"random": {Source: "hashicorp/random"},
			},
			expectedProvider: map[string][]map[string]any{
				"azurerm": {{"alias": "custom", "subscription_id": "sub"}},
			},
		},
		{
			desc:      "no provider configuration is generated",
			envConfig: &recipes.Configuration{},
			requiredProviders: map[string]*RequiredProviderInfo{
				"random": {Source: "hashicorp/random"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			envRecipe, resourceRecipe := getTestInputs()
			tfconfig, err := New(t.Context(), testRecipeName, &envRecipe, &resourceRecipe)
			require.NoError(t, err)

			// Simulate the placeholders that are added to retry the module download.
			require.True(t, tfconfig.AddProviderAliasPlaceholders(aliasEnvConfig, map[string]*RequiredProviderInfo{
				providers.AzureProviderName: {Source: "hashicorp/azurerm", ConfigurationAliases: []string{"azurerm.custom"}},
			}))
			require.NotNil(t, tfconfig.Provider)
			require.Contains(t, tfconfig.Module[testRecipeName], moduleProvidersKey)

			err = tfconfig.AddProviders(t.Context(), tc.requiredProviders, map[string]providers.Provider{}, tc.envConfig, nil)
			require.NoError(t, err)

			require.Equal(t, tc.expectedProvider, tfconfig.Provider)
			moduleProviders, ok := tfconfig.Module[testRecipeName][moduleProvidersKey]
			if tc.expectedProviders == nil {
				require.False(t, ok)
				return
			}
			require.Equal(t, tc.expectedProviders, moduleProviders)
		})
	}
}

func Test_AddProviders_CustomAzureConfiguration(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		secrets map[string]recipes.SecretData
		err     string
	}{
		{"resolved secrets", map[string]recipes.SecretData{"settings": {Data: map[string]string{"tenant": "test-tenant"}}}, ""},
		{"missing source", nil, "missing secret store id: settings"},
		{"missing key", map[string]recipes.SecretData{"settings": {}}, "missing secret key in secret store id: settings"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			envRecipe, resourceRecipe := getTestInputs()
			cfg, err := New(t.Context(), testRecipeName, &envRecipe, &resourceRecipe)
			require.NoError(t, err)
			env := &recipes.Configuration{RecipeConfig: datamodel.RecipeConfigProperties{
				Terraform: datamodel.TerraformConfigProperties{Providers: map[string][]datamodel.ProviderConfigProperties{
					"azurerm": {{
						AdditionalProperties: map[string]any{
							"alias": "custom", "subscription_id": "test-sub", "features": map[string]any{}, "use_cli": false,
						},
						Secrets: map[string]datamodel.SecretReference{"tenant_id": {Source: "settings", Key: "tenant"}},
					}},
				}},
			}}
			required := map[string]*RequiredProviderInfo{
				"azurerm": {Source: "hashicorp/azurerm", Version: "~> 3.114.0", ConfigurationAliases: []string{"azurerm.custom"}},
			}
			// An environment override must not fall back to the registered provider.
			fallback := providers.NewMockProvider(gomock.NewController(t))
			err = cfg.AddProviders(t.Context(), required, map[string]providers.Provider{"azurerm": fallback}, env, tc.secrets)
			if tc.err != "" {
				require.EqualError(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, map[string][]map[string]any{"azurerm": {{
				"alias": "custom", "subscription_id": "test-sub", "features": map[string]any{},
				"use_cli": false, "tenant_id": "test-tenant",
			}}}, cfg.Provider)
			require.Equal(t, map[string]string{"azurerm.custom": "azurerm.custom"}, cfg.Module[testRecipeName]["providers"])
			require.Equal(t, required, cfg.Terraform.RequiredProviders)

			dir := t.TempDir()
			require.NoError(t, cfg.Save(t.Context(), dir))
			data, err := os.ReadFile(getMainConfigFilePath(dir))
			require.NoError(t, err)
			var saved TerraformConfig
			require.NoError(t, json.Unmarshal(data, &saved))
			require.Equal(t, cfg.Provider, saved.Provider)
			require.Equal(t, map[string]any{"azurerm.custom": "azurerm.custom"}, saved.Module[testRecipeName]["providers"])
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
			tfconfig, err := New(t.Context(), testRecipeName, &envRecipe, &resourceRecipe)
			require.NoError(t, err)

			err = tfconfig.AddOutputs(tc.moduleName)
			if tc.expectedErr {
				require.Error(t, err)
				require.Nil(t, tfconfig.Output)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedOutputConfig, tfconfig.Output)
			}

			workingDir := t.TempDir()
			err = tfconfig.Save(t.Context(), workingDir)
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

func Test_AddMappedOutputs(t *testing.T) {
	envRecipe, resourceRecipe := getTestInputs()

	tests := []struct {
		desc           string
		moduleName     string
		outputsMap     map[string]string
		sensitivity    map[string]bool
		forceSensitive bool
		expectedOutput map[string]any
		expectedErr    bool
	}{
		{
			desc:       "maps non-sensitive and sensitive outputs",
			moduleName: testRecipeName,
			outputsMap: map[string]string{
				"host":     "endpoint",
				"password": "secret",
			},
			sensitivity: map[string]bool{
				"endpoint": false,
				"secret":   true,
			},
			expectedOutput: map[string]any{
				"endpoint": map[string]any{
					"value":     "${module." + testRecipeName + ".endpoint}",
					"sensitive": false,
				},
				"secret": map[string]any{
					"value":     "${module." + testRecipeName + ".secret}",
					"sensitive": true,
				},
			},
		},
		{
			desc:       "forceSensitive marks a non-sensitive module output sensitive",
			moduleName: testRecipeName,
			outputsMap: map[string]string{
				"connectionString": "primaryConnectionString",
			},
			// The module did not mark this output sensitive (mirrors AVM primaryConnectionString).
			sensitivity:    map[string]bool{"primaryConnectionString": false},
			forceSensitive: true,
			expectedOutput: map[string]any{
				"primaryConnectionString": map[string]any{
					"value":     "${module." + testRecipeName + ".primaryConnectionString}",
					"sensitive": true,
				},
			},
		},
		{
			desc:       "duplicate module outputs are de-duplicated",
			moduleName: testRecipeName,
			outputsMap: map[string]string{
				"primary":   "endpoint",
				"secondary": "endpoint",
			},
			sensitivity: map[string]bool{"endpoint": false},
			expectedOutput: map[string]any{
				"endpoint": map[string]any{
					"value":     "${module." + testRecipeName + ".endpoint}",
					"sensitive": false,
				},
			},
		},
		{
			desc:           "empty outputs map is a no-op",
			moduleName:     testRecipeName,
			outputsMap:     map[string]string{},
			expectedOutput: nil,
		},
		{
			desc:        "empty module name returns error",
			moduleName:  "",
			outputsMap:  map[string]string{"host": "endpoint"},
			expectedErr: true,
		},
		{
			desc:        "empty module output name returns error",
			moduleName:  testRecipeName,
			outputsMap:  map[string]string{"host": ""},
			expectedErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			tfconfig, err := New(t.Context(), testRecipeName, &envRecipe, &resourceRecipe)
			require.NoError(t, err)

			err = tfconfig.AddMappedOutputs(tc.moduleName, tc.outputsMap, tc.sensitivity, tc.forceSensitive)
			if tc.expectedErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expectedOutput, tfconfig.Output)
		})
	}
}

func Test_AddAllOutputs(t *testing.T) {
	envRecipe, resourceRecipe := getTestInputs()

	tests := []struct {
		desc           string
		moduleName     string
		sensitivity    map[string]bool
		expectedOutput map[string]any
		expectedErr    bool
	}{
		{
			desc:       "re-exports every module output preserving sensitivity",
			moduleName: testRecipeName,
			sensitivity: map[string]bool{
				"endpoint": false,
				"secret":   true,
			},
			expectedOutput: map[string]any{
				"endpoint": map[string]any{
					"value":     "${module." + testRecipeName + ".endpoint}",
					"sensitive": false,
				},
				"secret": map[string]any{
					"value":     "${module." + testRecipeName + ".secret}",
					"sensitive": true,
				},
			},
		},
		{
			desc:           "no module outputs is a no-op",
			moduleName:     testRecipeName,
			sensitivity:    map[string]bool{},
			expectedOutput: nil,
		},
		{
			desc:        "empty module name returns error",
			moduleName:  "",
			sensitivity: map[string]bool{"endpoint": false},
			expectedErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			tfconfig, err := New(t.Context(), testRecipeName, &envRecipe, &resourceRecipe)
			require.NoError(t, err)

			err = tfconfig.AddAllOutputs(tc.moduleName, tc.sensitivity)
			if tc.expectedErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expectedOutput, tfconfig.Output)
		})
	}
}

func Test_updateModuleWithProviderAliases(t *testing.T) {
	tests := []struct {
		name               string
		cfg                *TerraformConfig
		expectedConfig     *TerraformConfig
		requiredProviders  map[string]*RequiredProviderInfo
		expectedConfigFile string
		wantErr            bool
	}{
		{
			name: "Test with valid provider config",
			cfg: &TerraformConfig{
				Provider: map[string][]map[string]any{
					"aws": {
						{
							"alias":  "alias1",
							"region": "us-west-2",
						},
						{
							"alias":  "alias2",
							"region": "us-east-1",
						},
					},
				},
				Module: map[string]TFModuleConfig{
					"redis-azure": map[string]any{
						"redis_cache_name":    "redis-test",
						"resource_group_name": "test-rg",
						"sku":                 "P",
						"source":              "Azure/redis/azurerm",
						"version":             "1.1.0",
					},
				},
			},
			requiredProviders: map[string]*RequiredProviderInfo{
				providers.AWSProviderName: {
					Source:               "hashicorp/aws",
					Version:              ">= 3.0",
					ConfigurationAliases: []string{"aws.alias1", "aws.alias2"},
				},
			},
			expectedConfig: &TerraformConfig{
				Provider: map[string][]map[string]any{
					"aws": {
						{
							"alias":  "alias1",
							"region": "us-west-2",
						},
						{
							"alias":  "alias2",
							"region": "us-east-1",
						},
					},
				},
				Module: map[string]TFModuleConfig{
					"redis-azure": map[string]any{
						"redis_cache_name":    "redis-test",
						"resource_group_name": "test-rg",
						"sku":                 "P",
						"source":              "Azure/redis/azurerm",
						"version":             "1.1.0",
						"providers": map[string]string{
							"aws.alias1": "aws.alias1",
							"aws.alias2": "aws.alias2",
						},
					},
				},
			},
			expectedConfigFile: "testdata/providers-modules-aliases.tf.json",
			wantErr:            false,
		},
		{
			name: "Test with subset of required_provider aliases in provider config",
			cfg: &TerraformConfig{
				Provider: map[string][]map[string]any{
					"aws": {
						{
							"alias":  "alias1",
							"region": "us-west-2",
						},
					},
				},
				Module: map[string]TFModuleConfig{
					"redis-azure": map[string]any{
						"redis_cache_name":    "redis-test",
						"resource_group_name": "test-rg",
						"sku":                 "P",
						"source":              "Azure/redis/azurerm",
						"version":             "1.1.0",
					},
				},
			},
			requiredProviders: map[string]*RequiredProviderInfo{
				providers.AWSProviderName: {
					Source:               "hashicorp/aws",
					Version:              ">= 3.0",
					ConfigurationAliases: []string{"aws.alias1", "aws.alias2"},
				},
			},
			expectedConfig: &TerraformConfig{
				Provider: map[string][]map[string]any{
					"aws": {
						{
							"alias":  "alias1",
							"region": "us-west-2",
						},
					},
				},
				Module: map[string]TFModuleConfig{
					"redis-azure": map[string]any{
						"redis_cache_name":    "redis-test",
						"resource_group_name": "test-rg",
						"sku":                 "P",
						"source":              "Azure/redis/azurerm",
						"version":             "1.1.0",
						"providers": map[string]string{
							"aws.alias1": "aws.alias1",
						},
					},
				},
			},
			expectedConfigFile: "testdata/providers-modules-subsetaliases.tf.json",
			wantErr:            false,
		},
		{
			name: "Test with unmatched required_provider aliases in provider config",
			cfg: &TerraformConfig{
				Provider: map[string][]map[string]any{
					"aws": {
						{
							"region": "us-west-2",
						},
					},
				},
				Module: map[string]TFModuleConfig{
					"redis-azure": map[string]any{
						"redis_cache_name":    "redis-test",
						"resource_group_name": "test-rg",
						"sku":                 "P",
						"source":              "Azure/redis/azurerm",
						"version":             "1.1.0",
					},
				},
			},
			requiredProviders: map[string]*RequiredProviderInfo{
				providers.AWSProviderName: {
					Source:               "hashicorp/aws",
					Version:              ">= 3.0",
					ConfigurationAliases: []string{"aws.alias1"},
				},
			},
			expectedConfig: &TerraformConfig{
				Provider: nil,
				Module: map[string]TFModuleConfig{
					"redis-azure": map[string]any{
						"redis_cache_name":    "redis-test",
						"resource_group_name": "test-rg",
						"sku":                 "P",
						"source":              "Azure/redis/azurerm",
						"version":             "1.1.0",
					},
				},
			},
			expectedConfigFile: "testdata/providers-modules-unmatchedaliases.tf.json",
			wantErr:            false,
		},
		{
			name: "Test with no required_provider aliases",
			cfg: &TerraformConfig{
				Provider: map[string][]map[string]any{
					"aws": {
						{
							"alias":  "alias1",
							"region": "us-west-2",
						},
					},
				},
				Module: map[string]TFModuleConfig{
					"redis-azure": map[string]any{
						"redis_cache_name":    "redis-test",
						"resource_group_name": "test-rg",
						"sku":                 "P",
						"source":              "Azure/redis/azurerm",
						"version":             "1.1.0",
					},
				},
			},
			requiredProviders: map[string]*RequiredProviderInfo{
				providers.AWSProviderName: {
					Source:  "hashicorp/aws",
					Version: ">= 3.0",
				},
			},
			expectedConfig: &TerraformConfig{
				Provider: map[string][]map[string]any{
					"aws": {
						{
							"alias":  "alias1",
							"region": "us-west-2",
						},
					},
				},
				Module: map[string]TFModuleConfig{
					"redis-azure": map[string]any{
						"redis_cache_name":    "redis-test",
						"resource_group_name": "test-rg",
						"sku":                 "P",
						"source":              "Azure/redis/azurerm",
						"version":             "1.1.0",
					},
				},
			},
			expectedConfigFile: "testdata/providers-modules-noaliases.tf.json",
			wantErr:            false,
		},
		{
			name:              "TerraformConfig is nil",
			requiredProviders: nil,
			cfg:               nil,
			wantErr:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			err := tt.cfg.updateModuleWithProviderAliases(tt.requiredProviders)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			workingDir := t.TempDir()
			err = tt.cfg.Save(ctx, workingDir)
			require.NoError(t, err)

			// Assert generated config file matches expected config in JSON format.
			actualConfig, err := os.ReadFile(getMainConfigFilePath(workingDir))
			require.NoError(t, err)

			if tt.wantErr != true {
				expectedConfig, err := os.ReadFile(tt.expectedConfigFile)
				require.NoError(t, err)
				require.Equal(t, string(expectedConfig), string(actualConfig))
			}
		})
	}
}

func Test_Save_overwrite(t *testing.T) {
	ctx := t.Context()
	testDir := t.TempDir()
	envRecipe, resourceRecipe := getTestInputs()
	tfconfig, err := New(t.Context(), testRecipeName, &envRecipe, &resourceRecipe)
	require.NoError(t, err)

	err = tfconfig.Save(ctx, testDir)
	require.NoError(t, err)

	err = tfconfig.Save(ctx, testDir)
	require.NoError(t, err)
}

func Test_Save_ConfigFileReadOnly(t *testing.T) {
	testDir := t.TempDir()
	envRecipe, resourceRecipe := getTestInputs()
	tfconfig, err := New(t.Context(), testRecipeName, &envRecipe, &resourceRecipe)
	require.NoError(t, err)

	// Create a test configuration file with read only permission.
	err = os.WriteFile(getMainConfigFilePath(testDir), []byte(`{"module":{}}`), 0400)
	require.NoError(t, err)

	// Assert that Save returns an error.
	err = tfconfig.Save(t.Context(), testDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "permission denied")
}

func Test_Save_InvalidWorkingDir(t *testing.T) {
	testDir := filepath.Join("invalid", uuid.New().String())
	envRecipe, resourceRecipe := getTestInputs()

	tfconfig, err := New(t.Context(), testRecipeName, &envRecipe, &resourceRecipe)
	require.NoError(t, err)

	err = tfconfig.Save(t.Context(), testDir)
	require.Error(t, err)
	require.Equal(t, fmt.Sprintf("error creating file: open %s/main.tf.json: no such file or directory", testDir), err.Error())
}
