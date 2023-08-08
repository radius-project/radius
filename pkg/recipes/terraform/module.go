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

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/project-radius/radius/pkg/recipes/recipecontext"
)

const (
	moduleRootDir = ".terraform/modules"
)

// moduleInspectResult contains the result of inspecting a Terraform module config.
type moduleInspectResult struct {
	// ContextExists is true if the module contains a recipe context.
	ContextExists bool

	// RequiredProviders is a list of names of required providers for the module.
	RequiredProviders []string

	// We can add more inspection results here in the future.
}

// inspectTFModuleConfig inspects the module present at workingDir/.terraform/modules/<localModuleName> directory
// and returns the instpection result which includes the list of required providers and recipe context status.
// localModuleName is the name of the module specified in the configuration used to download the module.
// It uses terraform-config-inspect to load the module from the directory. An error is returned if the module
// could not be loaded.
func inspectTFModuleConfig(workingDir, localModuleName string) (*moduleInspectResult, error) {
	result := &moduleInspectResult{ContextExists: false, RequiredProviders: []string{}}

	// Modules are downloaded in a subdirectory in the working directory.
	// Name of the module specified in the configuration is used as subdirectory name.
	// https://developer.hashicorp.com/terraform/tutorials/modules/module-use#understand-how-modules-work
	mod, diags := tfconfig.LoadModule(filepath.Join(workingDir, moduleRootDir, localModuleName))
	if diags.HasErrors() {
		return nil, fmt.Errorf("error loading the module: %w", diags.Err())
	}

	// Ensure that the module has a recipe context.
	if _, ok := mod.Variables[recipecontext.RecipeContextParamKey]; ok {
		result.ContextExists = true
	}

	// Extract the list of required providers.
	for providerName := range mod.RequiredProviders {
		result.RequiredProviders = append(result.RequiredProviders, providerName)
	}

	return result, nil
}

// downloadModule downloads the module to the workingDir from the module source specified in the Terraform configuration.
// It uses Terraform's Get command to download the module using the Terraform executable available at execPath.
// An error is returned if the module could not be downloaded.
func downloadModule(ctx context.Context, workingDir, execPath string) error {
	tf, err := tfexec.NewTerraform(workingDir, execPath)
	if err != nil {
		return err
	}

	if err = tf.Get(ctx); err != nil {
		return err
	}

	return nil
}
