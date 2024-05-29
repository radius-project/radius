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
	"strings"
	"time"

	getter "github.com/hashicorp/go-getter"
	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/radius-project/radius/pkg/metrics"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/recipecontext"
	"github.com/radius-project/radius/pkg/recipes/terraform/config"
	"github.com/radius-project/radius/pkg/recipes/util"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	moduleRootDir = ".terraform/modules"
)

// moduleInspectResult contains the result of inspecting a Terraform module config.
type moduleInspectResult struct {
	// ContextVarExists is true if the module has a variable defined for recipe context.
	ContextVarExists bool

	// RequiredProviders is a map where the key is the name of required providers for the module,
	// and the value is a pointer to a RequiredProviderInfo struct that contains the details for the provider.
	RequiredProviders map[string]*config.RequiredProviderInfo

	// ResultOutputExists is true if the module contains an output named "result".
	ResultOutputExists bool

	// The parameter variables defined by the recipe
	Parameters map[string]any

	// Any other module information required in the future can be added here.
}

// downloadAndInspect handles downloading the TF module and retrieving the necessary information
func downloadAndInspect(ctx context.Context, tf *tfexec.Terraform, options Options) (*moduleInspectResult, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Run Terraform Get command to download the module from the source specified in the config.
	// The downloaded module is stored in the working directory.
	logger.Info(fmt.Sprintf("Downloading Terraform module: %s", options.EnvRecipe.TemplatePath))
	downloadStartTime := time.Now()
	if err := tf.Get(ctx); err != nil {
		metrics.DefaultRecipeEngineMetrics.RecordRecipeDownloadDuration(ctx, downloadStartTime,
			metrics.NewRecipeAttributes(metrics.RecipeEngineOperationDownloadRecipe, options.EnvRecipe.Name,
				options.EnvRecipe, recipes.RecipeDownloadFailed))

		errMsg := fmt.Sprintf("failed to download Terraform module from source %q, version %q: %s", options.EnvRecipe.TemplatePath, options.EnvRecipe.TemplateVersion, err.Error())
		return nil, recipes.NewRecipeError(recipes.RecipeDownloadFailed, errMsg, util.RecipeSetupError, recipes.GetErrorDetails(err))
	}

	metrics.DefaultRecipeEngineMetrics.RecordRecipeDownloadDuration(ctx, downloadStartTime,
		metrics.NewRecipeAttributes(metrics.RecipeEngineOperationDownloadRecipe, options.EnvRecipe.Name,
			options.EnvRecipe, metrics.SuccessfulOperationState))

	// Load the downloaded module to retrieve providers and variables required by the module.
	// This is needed to add the appropriate providers config and populate the value of recipe context variable.
	logger.Info(fmt.Sprintf("Inspecting the downloaded Terraform module: %s", options.EnvRecipe.TemplatePath))
	loadedModule, err := inspectModule(tf.WorkingDir(), options.EnvRecipe)
	if err != nil {
		return nil, err
	}

	return loadedModule, nil
}

// inspectModule inspects the module present at workingDir/.terraform/modules/<localModuleName> directory
// and returns the inspection result which includes the list of required provider names, existence of recipe context variable and result output.
// localModuleName is the name of the module specified in the configuration used to download the module.
// It uses terraform-config-inspect to load the module from the directory. An error is returned if the module
// could not be loaded.
func inspectModule(workingDir string, recipe *recipes.EnvironmentDefinition) (*moduleInspectResult, error) {
	result := &moduleInspectResult{ContextVarExists: false, RequiredProviders: map[string]*config.RequiredProviderInfo{}, ResultOutputExists: false, Parameters: map[string]any{}}

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

	// Extract the details of required providers.
	for k, v := range mod.RequiredProviders {
		requiredprovider := &config.RequiredProviderInfo{}

		requiredprovider.Source = v.Source
		if len(v.VersionConstraints) > 0 {
			requiredprovider.Version = strings.Join(v.VersionConstraints, ",")
		}

		if len(v.ConfigurationAliases) > 0 {
			var aliases []string
			for _, alias := range v.ConfigurationAliases {
				// Concatenate Name and Alias from ProviderRef. This is expected format of provider alias in required_provider configuration_aliases field.
				// Ref: https://developer.hashicorp.com/terraform/language/modules/develop/providers#provider-aliases-within-modules
				if alias.Name != "" && alias.Alias != "" {
					aliases = append(aliases, alias.Name+"."+alias.Alias)
				}
			}
			if len(aliases) > 0 {
				requiredprovider.ConfigurationAliases = aliases
			}
		}

		result.RequiredProviders[k] = requiredprovider
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
