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

package terraform

import (
	"context"
	"fmt"
	"path/filepath"

	getter "github.com/hashicorp/go-getter"
	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/recipecontext"
)

const (
	moduleRootDir = ".terraform/modules"
)

// moduleInspectResult contains the result of inspecting a Terraform module config.
type moduleInspectResult struct {
	// ContextVarExists is true if the module has a variable defined for recipe context.
	ContextVarExists bool

	// RequiredProviders is a list of names of required providers for the module.
	RequiredProviders []string

	// ResultOutputExists is true if the module contains an output named "result".
	ResultOutputExists bool

	// The parameter variables defined by the recipe
	Parameters map[string]any

	// Any other module information required in the future can be added here.
}

// inspectModule inspects the module present at workingDir/.terraform/modules/<localModuleName> directory
// and returns the inspection result which includes the list of required provider names, existence of recipe context variable and result output.
// localModuleName is the name of the module specified in the configuration used to download the module.
// It uses terraform-config-inspect to load the module from the directory. An error is returned if the module
// could not be loaded.
func inspectModule(workingDir string, recipe *recipes.EnvironmentDefinition) (*moduleInspectResult, error) {
	result := &moduleInspectResult{ContextVarExists: false, RequiredProviders: []string{}, ResultOutputExists: false, Parameters: map[string]any{}}

	// Modules are downloaded in a subdirectory in the working directory.
	// Name of the module specified in the configuration is used as subdirectory name.
	// https://developer.hashicorp.com/terraform/tutorials/modules/module-use#understand-how-modules-work
	//
	// If the template path is for a submodule, we'll add the submodule path to the module directory.
	_, subModule := getter.SourceDirSubdir(recipe.TemplatePath)
	mod, diags := tfconfig.LoadModule(filepath.Join(workingDir, moduleRootDir, recipe.Name, subModule))
	if diags.HasErrors() {
		return nil, fmt.Errorf("error loading the module: %w", diags.Err())
	}

	// Check that the module has a recipe context variable.
	if _, ok := mod.Variables[recipecontext.RecipeContextParamKey]; ok {
		result.ContextVarExists = true
	}

	// Extract the list of required providers.
	for providerName := range mod.RequiredProviders {
		result.RequiredProviders = append(result.RequiredProviders, providerName)
	}

	// Check if an output named "result" is defined in the module.
	if _, ok := mod.Outputs[recipes.ResultPropertyName]; ok {
		result.ResultOutputExists = true
	}

	// Extract the list of parameters.
	for variable, value := range mod.Variables {
		tfVar := map[string]any{
			"name":         value.Name,
			"type":         value.Type,
			"description":  value.Description,
			"defaultValue": value.Default,
			"required":     value.Required,
			"sensitive":    value.Sensitive,
			"pos":          value.Pos,
		}
		result.Parameters[variable] = tfVar
	}

	return result, nil
}

// downloadModule downloads the module to the workingDir from the module source specified in the Terraform configuration.
// It uses Terraform's Get command to download the module using the Terraform executable available at execPath.
// An error is returned if the module could not be downloaded.
func downloadModule(ctx context.Context, tf *tfexec.Terraform, templatePath string) error {
	if err := tf.Get(ctx); err != nil {
		return fmt.Errorf("failed to run terraform get to download the module from source %q: %w", templatePath, err)
	}

	return nil
}
