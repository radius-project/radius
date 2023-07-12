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
)

// Generate Terraform configuration in JSON format for required providers and modules, and write it
// to a file in the specified working directory. This JSON configuration is needed to initialize
// and apply Terraform modules. See https://www.terraform.io/docs/language/syntax/json.html
// for more information on the JSON syntax for Terraform configuration.
func GenerateMainConfigFile(ctx context.Context, envRecipe *recipes.EnvironmentDefinition, resourceRecipe *recipes.ResourceMetadata, workingDir string) error {
	moduleData := generateModuleData(ctx, envRecipe.TemplatePath, envRecipe.Parameters, resourceRecipe.Parameters)

	tfConfig := TerraformConfig{
		Module: map[string]any{
			envRecipe.Name: moduleData,
		},
	}

	// Convert the Terraform config to JSON
	jsonData, err := json.MarshalIndent(tfConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshalling JSON: %w", err)
	}

	// Write the JSON data to a file in the working directory.
	// JSON configuration syntax for Terraform requires the file to be named with .tf.json suffix.
	// https://developer.hashicorp.com/terraform/language/syntax/json
	configFilePath := fmt.Sprintf("%s/%s", workingDir, mainConfigFileName)
	file, err := os.Create(configFilePath)
	if err != nil {
		return fmt.Errorf("error creating file: %w", err)
	}
	defer file.Close()

	_, err = file.Write(jsonData)
	if err != nil {
		return fmt.Errorf("error writing to file: %w", err)
	}

	return nil
}

func generateModuleData(ctx context.Context, moduleSource string, envParams, resourceParams map[string]any) map[string]any {
	moduleConfig := map[string]any{
		moduleSourceKey: moduleSource,
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
