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
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/recipecontext"
	"github.com/project-radius/radius/pkg/recipes/terraform/config/backends"
	"github.com/project-radius/radius/pkg/recipes/terraform/config/providers"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

const (
	// modeConfigFile is read/write mode only for the owner of the TF config file.
	modeConfigFile fs.FileMode = 0600
)

var ErrModuleNotFound = errors.New("module not found in Terraform config")

// New creates TerraformConfig with the given module name and its inputs (module source, version, parameters)
// from environment recipe and resource recipe metadata.
func New(moduleName string, envRecipe *recipes.EnvironmentDefinition, resourceRecipe *recipes.ResourceMetadata) *TerraformConfig {
	// Resource parameter gets precedence over environment level parameter,
	// if same parameter is defined in both environment and resource recipe metadata.
	moduleData := newModuleConfig(envRecipe.TemplatePath, envRecipe.TemplateVersion, envRecipe.Parameters, resourceRecipe.Parameters)

	return &TerraformConfig{
		Terraform: nil,
		Provider:  nil,
		Module: map[string]TFModuleConfig{
			moduleName: moduleData,
		},
	}
}

// getMainConfigFilePath returns the path of the Terraform main config file.
func getMainConfigFilePath(workingDir string) string {
	return fmt.Sprintf("%s/%s", workingDir, mainConfigFileName)
}

// Save writes the Terraform config to the main config file present at ConfigFilePath().
// This overwrites the existing file if it exists.
func (cfg *TerraformConfig) Save(ctx context.Context, workingDir string) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Write the JSON data to a file in the working directory.
	// JSON configuration syntax for Terraform requires the file to be named with .tf.json suffix.
	// https://developer.hashicorp.com/terraform/language/syntax/json

	// Convert the Terraform config to JSON
	jsonData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshalling JSON: %w", err)
	}

	logger.Info(fmt.Sprintf("Writing Terraform JSON config to file: %s", getMainConfigFilePath(workingDir)))
	if err = os.WriteFile(getMainConfigFilePath(workingDir), jsonData, modeConfigFile); err != nil {
		return fmt.Errorf("error creating file: %w", err)
	}
	return nil
}

// AddProviders adds provider configurations for requiredProviders that are supported
// by Radius to generate custom provider configurations. Save() must be called to save
// the generated providers config. requiredProviders contains a list of provider names
// that are required for the module.
func (cfg *TerraformConfig) AddProviders(ctx context.Context, requiredProviders []string, supportedProviders map[string]providers.Provider, envConfig *recipes.Configuration) error {
	providerConfigs, err := getProviderConfigs(ctx, requiredProviders, supportedProviders, envConfig)
	if err != nil {
		return err
	}

	// Add generated provider configs for required providers to the existing terraform json config file
	if len(providerConfigs) > 0 {
		cfg.Provider = providerConfigs
	}

	return nil
}

// AddRecipeContext adds RecipeContext to TerraformConfig module parameters if recipeCtx is not nil.
// Save() must be called after adding recipe context to the module config.
func (cfg *TerraformConfig) AddRecipeContext(ctx context.Context, moduleName string, recipeCtx *recipecontext.Context) error {
	mod, ok := cfg.Module[moduleName]
	if !ok {
		// must not happen because module key is set in New().
		panic(ErrModuleNotFound)
	}
	if recipeCtx != nil {
		mod.SetParams(RecipeParams{recipecontext.RecipeContextParamKey: recipeCtx})
	}
	return nil
}

// newModuleConfig creates a new TFModuleConfig object with the given module source and version
// and also populates RecipeParams in TF module config. If same parameter key exists across params
// then the last map specified gets precedence.
func newModuleConfig(moduleSource string, moduleVersion string, params ...RecipeParams) TFModuleConfig {
	moduleConfig := TFModuleConfig{
		moduleSourceKey: moduleSource,
	}

	// Not all sources use versions, so only add the version if it's specified.
	// Registries require versions, but HTTP or filesystem sources do not.
	if moduleVersion != "" {
		moduleConfig[moduleVersionKey] = moduleVersion
	}

	// Populate recipe parameters
	for _, param := range params {
		moduleConfig.SetParams(param)
	}

	return moduleConfig
}

// getProviderConfigs generates the Terraform provider configurations for the required providers.
func getProviderConfigs(ctx context.Context, requiredProviders []string, supportedProviders map[string]providers.Provider, envConfig *recipes.Configuration) (map[string]any, error) {
	providerConfigs := make(map[string]any)
	for _, provider := range requiredProviders {
		builder, ok := supportedProviders[provider]
		if !ok {
			// No-op: For any other provider, Radius doesn't generate any custom configuration.
			continue
		}

		config, err := builder.BuildConfig(ctx, envConfig)
		if err != nil {
			return nil, err
		}
		if len(config) > 0 {
			providerConfigs[provider] = config
		}
	}

	return providerConfigs, nil
}

// AddBackend adds backend configurations to store Terraform state file for the deployment.
// Save() must be called to save the generated backend config.
// Currently, the supported backend for Terraform Recipes is Kubernetes secret. https://developer.hashicorp.com/terraform/language/settings/backends/kubernetes
func (cfg *TerraformConfig) AddBackend(resourceRecipe *recipes.ResourceMetadata, backend backends.Backend) (string, error) {
	backendConfig, err := backend.BuildBackend(resourceRecipe)
	if err != nil {
		return "", err
	}
	cfg.Terraform = &TerraformDefinition{
		Backend: backendConfig,
	}
	var secretSuffix string
	if backendDetails, ok := backendConfig["kubernetes"]; ok {
		backendMap := backendDetails.(map[string]any)
		if secret, ok := backendMap["secret"]; ok {
			secretSuffix = secret.(string)
		}
	}
	return secretSuffix, nil
}
