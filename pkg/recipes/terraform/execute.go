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
	"errors"
	"fmt"
	"os"
	"path/filepath"

	install "github.com/hashicorp/hc-install"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/recipecontext"
	"github.com/project-radius/radius/pkg/recipes/terraform/config"
	"github.com/project-radius/radius/pkg/recipes/terraform/config/providers"
	"github.com/project-radius/radius/pkg/sdk"
	ucp_provider "github.com/project-radius/radius/pkg/ucp/secret/provider"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

const (
	executionSubDir = "deploy"
)

var (
	// ErrRecipeNameEmpty is the error when the recipe name is empty.
	ErrRecipeNameEmpty = errors.New("recipe name cannot be empty")
)

var _ TerraformExecutor = (*executor)(nil)

// NewExecutor creates a new Executor with the given UCP connection and secret provider, to execute a Terraform recipe.
func NewExecutor(ucpConn sdk.Connection, secretProvider *ucp_provider.SecretProvider) *executor {
	return &executor{ucpConn: ucpConn, secretProvider: secretProvider}
}

type executor struct {
	// ucpConn represents the configuration needed to connect to UCP, required to fetch cloud provider credentials.
	ucpConn sdk.Connection

	// secretProvider is the secret store provider used for managing credentials in UCP.
	secretProvider *ucp_provider.SecretProvider
}

// # Function Explanation
//
// Deploy installs Terraform, creates a working directory, generates a config, and runs Terraform init and
// apply in the working directory, returning an error if any of these steps fail.
func (e *executor) Deploy(ctx context.Context, options Options) (*recipes.RecipeOutput, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Install Terraform
	i := install.NewInstaller()
	execPath, err := Install(ctx, i, options.RootDir)
	// The terraform zip for installation is downloaded in a location outside of the install directory and is only accessible through the installer.Remove function -
	// stored in latestVersion.pathsToRemove. So this needs to be called for complete cleanup even if the root terraform directory is deleted.
	defer func() {
		if err := i.Remove(ctx); err != nil {
			logger.Info(fmt.Sprintf("Failed to cleanup Terraform installation: %s", err.Error()))
		}
	}()
	if err != nil {
		return nil, err
	}

	// Create Working Directory
	workingDir, err := createWorkingDir(ctx, options.RootDir)
	if err != nil {
		return nil, err
	}

	// Create Terraform config in the working directory
	err = e.generateConfig(ctx, workingDir, execPath, options)
	if err != nil {
		return nil, err
	}

	// Run TF Init and Apply in the working directory
	err = initAndApply(ctx, workingDir, execPath)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func createWorkingDir(ctx context.Context, tfDir string) (string, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	workingDir := filepath.Join(tfDir, executionSubDir)
	logger.Info(fmt.Sprintf("Creating Terraform working directory: %q", workingDir))
	if err := os.MkdirAll(workingDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create working directory for terraform execution: %w", err)
	}

	return workingDir, nil
}

// generateConfig generates Terraform configuration with required inputs for the module to be initialized and applied.
func (e *executor) generateConfig(ctx context.Context, workingDir, execPath string, options Options) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Generate Terraform json config in the working directory
	// Use recipe name as a local reference to the module.
	// Modules are downloaded in a subdirectory in the working directory. Name of the module specified in the
	// configuration is used as subdirectory name under .terraform/modules directory.
	// https://developer.hashicorp.com/terraform/tutorials/modules/module-use#understand-how-modules-work
	localModuleName := options.EnvRecipe.Name
	if localModuleName == "" {
		return ErrRecipeNameEmpty
	}

	// Create a new Terraform JSON config with the given recipe parameters and working directory.
	tfConfig := config.New(localModuleName, options.EnvRecipe, options.ResourceRecipe)

	// Before downloading module, Teraform configuration needs to be saved because downloading module
	// requires the default config.
	if err := tfConfig.Save(ctx, workingDir); err != nil {
		return err
	}

	logger.Info(fmt.Sprintf("Downloading recipe module: %s", options.ResourceRecipe.Name))
	// Download module in the working directory.
	if err := downloadModule(ctx, workingDir, execPath); err != nil {
		return err
	}

	logger.Info(fmt.Sprintf("Inspecting downloaded recipe: %s", options.ResourceRecipe.Name))
	// Get the inspection result from downloaded module to extract recipecontext existency and providers.
	result, err := inspectTFModuleConfig(workingDir, localModuleName)
	if err != nil {
		logger.Error(err, "Failed to inspect module")
		return err
	}

	logger.Info(fmt.Sprintf("Inspected module result: %+v", result))
	// Add the required providers to the terraform configuration.
	if err := tfConfig.AddProviders(ctx, result.Providers, providers.GetSupportedTerraformProviders(e.ucpConn, e.secretProvider),
		options.EnvConfig); err != nil {
		return err
	}

	// Populate recipecontext into TF JSON config only if the downloaded module has a context variable.
	if result.ContextExists {
		// create the context object to be passed to the recipe deployment
		recipectx, err := recipecontext.New(options.ResourceRecipe, options.EnvConfig)
		if err != nil {
			return err
		}
		if err = tfConfig.AddRecipeContext(ctx, localModuleName, recipectx); err != nil {
			return err
		}
	}

	// Add more configurations here.

	// Ensure that we need to save the configuration after adding providers and recipecontext.
	if err := tfConfig.Save(ctx, workingDir); err != nil {
		return err
	}

	return nil
}

// initAndApply runs Terraform init and apply in the provided working directory.
func initAndApply(ctx context.Context, workingDir, execPath string) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	tf, err := tfexec.NewTerraform(workingDir, execPath)
	if err != nil {
		return err
	}

	// Initialize Terraform
	logger.Info("Initializing Terraform")
	if err := tf.Init(ctx); err != nil {
		return fmt.Errorf("terraform init failure: %w", err)
	}

	// Apply Terraform configuration
	logger.Info("Running Terraform apply")
	if err := tf.Apply(ctx); err != nil {
		return fmt.Errorf("terraform apply failure: %w", err)
	}

	return nil
}
