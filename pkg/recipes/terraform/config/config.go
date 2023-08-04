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
	"fmt"
	"os"

	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/recipecontext"
	"github.com/project-radius/radius/pkg/recipes/terraform/config/providers"
)

// # Function Explanation
//
// GenerateTFConfigFile generates Terraform configuration in JSON format with module inputs, and writes it
// to a main.tf.json file in the specified working directory. This JSON configuration is needed to retrieve the Terraform
// module referenced by the Recipe. See https://www.terraform.io/docs/language/syntax/json.html
// for more information on the JSON syntax for Terraform configuration.
// Returns path to the generated config file.
func GenerateTFConfigFile(ctx context.Context, workingDir, localModuleName string, envRecipe *recipes.EnvironmentDefinition, resourceRecipe *recipes.ResourceMetadata, recieptctx *recipecontext.RecipeContext) (string, error) {
	moduleData := generateModuleData(ctx, envRecipe.TemplatePath, envRecipe.TemplateVersion, envRecipe.Parameters, resourceRecipe.Parameters)

	// Populate recipe context to module data.
	moduleData[ModuleRecipeContextKey] = recieptctx

	tfConfig := TerraformConfig{
		Module: map[string]any{
			localModuleName: moduleData,
		},
	}

	// Convert the Terraform config to JSON
	jsonData, err := json.MarshalIndent(tfConfig, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error marshalling JSON: %w", err)
	}

	// Write the JSON data to a file in the working directory.
	// JSON configuration syntax for Terraform requires the file to be named with .tf.json suffix.
	// https://developer.hashicorp.com/terraform/language/syntax/json
	configFilePath := fmt.Sprintf("%s/%s", workingDir, mainConfigFileName)
	file, err := os.Create(configFilePath)
	if err != nil {
		return "", fmt.Errorf("error creating file: %w", err)
	}
	defer file.Close()

	_, err = file.Write(jsonData)
	if err != nil {
		return "", fmt.Errorf("error writing to file: %w", err)
	}

	return configFilePath, nil
}

func generateModuleData(ctx context.Context, moduleSource string, moduleVersion string, envParams, resourceParams map[string]any) map[string]any {
	moduleConfig := map[string]any{
		moduleSourceKey: moduleSource,
	}

	// Not all sources use versions, so only add the version if it's specified.
	// Registries require versions, but HTTP or filesystem sources do not.
	if moduleVersion != "" {
		moduleConfig[moduleVersionKey] = moduleVersion
	}

	// Populate recipe parameters
	// Resource parameter gets precedence over environment level parameter,
	// if same parameter is defined in both environment and resource recipe metadata.
	for key, value := range envParams {
		moduleConfig[key] = value
	}

	for key, value := range resourceParams {
		moduleConfig[key] = value
	}

	return moduleConfig
}

// # Function Explanation
//
// AddProviders generates and adds provider configurations for requiredProviders that are supported by Radius to generate custom provider configurations.
// The generated config is added to the existing Terraform main config file present at the configFilePath, and writes the updated configuration data back to the file.
// requiredProviders contains a list of provider names that are required for the module.
func AddProviders(ctx context.Context, configFilePath string, requiredProviders []string, supportedProviders map[string]providers.Provider, envConfig *recipes.Configuration) error {
	providerConfigs, err := getProviderConfigs(ctx, requiredProviders, supportedProviders, envConfig)
	if err != nil {
		return err
	}

	// Add generated provider configs for required providers to the existing terraform json config file
	if len(providerConfigs) > 0 {
		configFile, err := os.Open(configFilePath)
		if err != nil {
			return fmt.Errorf("error opening file %q: %w", configFilePath, err)
		}
		defer configFile.Close()

		var tfConfig TerraformConfig
		err = json.NewDecoder(configFile).Decode(&tfConfig)
		if err != nil {
			return err
		}

		tfConfig.Provider = providerConfigs

		// Write the updated config data to the Terraform json config file
		updatedConfig, err := json.MarshalIndent(tfConfig, "", "  ")
		if err != nil {
			return err
		}
		err = os.WriteFile(configFilePath, updatedConfig, 0666)
		if err != nil {
			return err
		}
	}

	return nil
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
