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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/recipecontext"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/backends"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/providers"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	// modeConfigFile is read/write mode only for the owner of the TF config file.
	modeConfigFile fs.FileMode = 0600
)

// New creates TerraformConfig with the given module name and its inputs (module source, version, parameters)
// Parameters are populated from environment recipe and resource recipe metadata.
func New(ctx context.Context, moduleName string, envRecipe *recipes.EnvironmentDefinition, resourceRecipe *recipes.ResourceMetadata, envConfig *recipes.Configuration) (*TerraformConfig, error) {
	path := envRecipe.TemplatePath

	if envConfig != nil {
		// Retrieving the secret store with associated with the template path.
		// appends an URL prefix to the templatePath if secret store exists.
		secretStore, err := recipes.GetSecretStoreID(*envConfig, envRecipe.TemplatePath)
		if err != nil {
			return nil, err
		}

		if secretStore != "" {
			// Retrieving the URL prefix, prefix will be in the format of https://<environment>-<application>-<resource>-
			prefix, err := recipes.GetURLPrefix(resourceRecipe)
			if err != nil {
				return nil, err
			}

			url, err := recipes.GetGitURL(envRecipe.TemplatePath)
			if err != nil {
				return nil, err
			}

			// Adding URL prefix to the template path.
			// Adding the prefix helps to access the the right credential information for git across environments.
			// Updated template path will be added to the terraform config.
			path = fmt.Sprintf("git::%s%s", prefix, strings.TrimPrefix(url.String(), "https://"))
		}
	}

	// Resource parameter gets precedence over environment level parameter,
	// if same parameter is defined in both environment and resource recipe metadata.
	moduleData := newModuleConfig(path, envRecipe.TemplateVersion, envRecipe.Parameters, resourceRecipe.Parameters)

	return &TerraformConfig{
		Terraform: nil,
		Provider:  nil,
		Module: map[string]TFModuleConfig{
			moduleName: moduleData,
		},
	}, nil
}

// getMainConfigFilePath returns the path of the Terraform main config file.
func getMainConfigFilePath(workingDir string) string {
	return fmt.Sprintf("%s/%s", workingDir, mainConfigFileName)
}

// Save writes the Terraform config to main.tf.json file in the working directory.
// This overwrites the existing file if it exists.
func (cfg *TerraformConfig) Save(ctx context.Context, workingDir string) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Write the JSON data to a file in the working directory.
	// JSON configuration syntax for Terraform requires the file to be named with .tf.json suffix.
	// https://developer.hashicorp.com/terraform/language/syntax/json

	// Create a buffer to write the JSON to
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")

	// Encode the Terraform config to JSON
	if err := enc.Encode(cfg); err != nil {
		return fmt.Errorf("error marshalling JSON: %w", err)
	}

	// Remove trailing newline
	jsonData := strings.TrimSuffix(buf.String(), "\n")

	logger.Info(fmt.Sprintf("Writing Terraform JSON config to file: %s", getMainConfigFilePath(workingDir)))
	if err := os.WriteFile(getMainConfigFilePath(workingDir), []byte(jsonData), modeConfigFile); err != nil {
		return fmt.Errorf("error creating file: %w", err)
	}
	return nil
}

// AddProviders adds provider configurations for requiredProviders that are supported
// by Radius to generate custom provider configurations. Save() must be called to save
// the generated providers config. requiredProviders contains a list of provider names
// that are required for the module.
func (cfg *TerraformConfig) AddProviders(ctx context.Context, requiredProviders map[string]*RequiredProviderInfo, ucpConfiguredProviders map[string]providers.Provider, envConfig *recipes.Configuration) error {
	providerConfigs, err := getProviderConfigs(ctx, requiredProviders, ucpConfiguredProviders, envConfig)
	if err != nil {
		return err
	}

	// Add generated provider configs for required providers to the existing terraform json config file
	if len(providerConfigs) > 0 {
		cfg.Provider = providerConfigs
	}

	return nil
}

// UpdateModuleProvidersWithAliases updates the module provider configuration in the Terraform config
// by adding aliases to the provider configurations.
func (cfg *TerraformConfig) UpdateModuleProvidersWithAliases(ctx context.Context) error {
	if cfg == nil {
		return fmt.Errorf("terraform configuration is not initialized")
	}

	providerConfigs := cfg.Provider
	moduleAliasConfig := map[string]string{}

	// For each provider in the providerConfigs, if provider has a property "alias",
	// add entry to the module provider configuration
	for providerName, providerConfigList := range providerConfigs {
		providerConfigDetails, ok := providerConfigList.([]map[string]any)
		if !ok {
			return fmt.Errorf("providerConfigList is not of type []map[string]any")
		}
		for _, providerConfig := range providerConfigDetails {
			if alias, ok := providerConfig["alias"]; ok {
				moduleAliasConfig[providerName+"."+fmt.Sprintf("%v", alias)] = providerName + "." + fmt.Sprintf("%v", alias)
			}
		}
	}

	// Update the module provider configuration in the Terraform config.
	if len(moduleAliasConfig) > 0 {
		moduleConfig := cfg.Module
		for _, module := range moduleConfig {
			module["providers"] = moduleAliasConfig
		}
	}

	return nil
}

// AddRecipeContext adds RecipeContext to TerraformConfig module parameters if recipeCtx is not nil.
// Save() must be called after adding recipe context to the module config.
func (cfg *TerraformConfig) AddRecipeContext(ctx context.Context, moduleName string, recipeCtx *recipecontext.Context) error {
	mod, ok := cfg.Module[moduleName]
	if !ok {
		// must not happen because module key is set when the config is initialized in New().
		return fmt.Errorf("module %q not found in the initialized terraform config", moduleName)
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

// getProviderConfigs generates the Terraform provider configurations. This is built from a combination of environment level recipe configuration for
// providers and the provider configurations registered with UCP. The environment level recipe configuration for providers takes precedence over UCP provider configurations.
func getProviderConfigs(ctx context.Context, requiredProviders map[string]*RequiredProviderInfo, ucpConfiguredProviders map[string]providers.Provider, envConfig *recipes.Configuration) (map[string]any, error) {
	// Get recipe provider configurations from the environment configuration
	providerConfigs := providers.GetRecipeProviderConfigs(ctx, envConfig)

	// Build provider configurations for required providers excluding the ones already present in providerConfigs
	for providerName := range requiredProviders {
		if _, ok := ucpConfiguredProviders[providerName]; ok { // requiredProviders can contain providers not configured with UCP
			if _, ok := providerConfigs[providerName]; ok {
				// Environment level recipe configuration for providers will take precedence over
				// UCP provider configuration (currently these include azurerm, aws, kubernetes providers)
				continue
			}
		} else {
			// If the provider under required_providers is not configured with UCP, skip this iteration.
			continue
		}

		builder, ok := ucpConfiguredProviders[providerName]
		if !ok {
			// No-op: For any other provider under required_providers, Radius doesn't generate any custom configuration.
			continue
		}

		config, err := builder.BuildConfig(ctx, envConfig)
		if err != nil {
			return nil, err
		}
		if len(config) > 0 {
			providerConfigs[providerName] = config
		}
	}

	return providerConfigs, nil
}

// AddTerraformInfrastructure adds backend configurations to store Terraform state file for the deployment.
// It also sets the required providers for the Terraform configuration.
// Save() must be called to save the generated backend config.
// Currently, the supported backend for Terraform Recipes is Kubernetes secret. https://developer.hashicorp.com/terraform/language/settings/backends/kubernetes
func (cfg *TerraformConfig) AddTerraformInfrastructure(resourceRecipe *recipes.ResourceMetadata, backend backends.Backend, requiredProviders map[string]*RequiredProviderInfo) (map[string]any, error) {
	backendConfig, err := backend.BuildBackend(resourceRecipe)
	if err != nil {
		return nil, err
	}

	cfg.Terraform = &TerraformDefinition{
		Backend:           backendConfig,
		RequiredProviders: requiredProviders,
	}

	return backendConfig, nil
}

// Add outputs to the config file referencing module outputs to populate expected Radius resource outputs.
// Outputs of modules are accessible through this format: module.<MODULE NAME>.<OUTPUT NAME>
// https://developer.hashicorp.com/terraform/language/modules/syntax#accessing-module-output-values
// This function only updates config in memory, Save() must be called to persist the updated config.
func (cfg *TerraformConfig) AddOutputs(localModuleName string) error {
	if localModuleName == "" {
		return errors.New("module name cannot be empty")
	}

	cfg.Output = map[string]any{
		recipes.ResultPropertyName: map[string]any{
			"value":     "${module." + localModuleName + "." + recipes.ResultPropertyName + "}",
			"sensitive": true, // since secret and non-secret values are combined in the result, mark the entire output sensitive
		},
	}

	return nil
}
