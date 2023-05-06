// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/releases"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/ucp/util"
)

const (
	installDirRoot = "/terraform/install"
	workingDirRoot = "/terraform/exec"
)

var _ Driver = (*terraformDriver)(nil)

// NewTerraformDriver creates a new instance of driver to execute a Terraform recipe.
func NewTerraformDriver() Driver {
	return &terraformDriver{}
}

type terraformDriver struct {
}

type TerraformConfig struct {
	Terraform TerraformDefinition    `json:"terraform"`
	Provider  map[string]interface{} `json:"provider"`
	Module    map[string]ModuleData  `json:"module"`
}

type TerraformDefinition struct {
	RequiredProviders map[string]interface{} `json:"required_providers"`
	RequiredVersion   string                 `json:"required_version"`
}

type ModuleData struct {
	Source  string `json:"source"`
	Version string `json:"version"`
}

func (d *terraformDriver) Execute(ctx context.Context, configuration recipes.Configuration, recipe recipes.Metadata, definition recipes.Definition) (recipeOutput *recipes.RecipeOutput, err error) {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("Deploying recipe: %q, template: %q", recipe.Name, definition.TemplatePath))

	// Create Terraform installation directory
	installDir := filepath.Join(installDirRoot, util.NormalizeStringToLower(recipe.ResourceID), uuid.NewString())
	logger.Info(fmt.Sprintf("Creating Terraform install directory: %q", installDir))
	if err = os.MkdirAll(installDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create directory for terraform installation for resource %q: %w", recipe.ResourceID, err)
	}
	defer os.RemoveAll(installDir)

	// Install Terraform
	// Using latest version, revisit this if we want to use a specific version.
	installer := &releases.LatestVersion{
		Product:    product.Terraform,
		InstallDir: installDir,
	}
	// We should look into checking if an exsiting installation of Terraform is available.
	// For initial iteration we will always install Terraform. Optimizations can be made later.
	execPath, err := installer.Install(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to install terraform for resource %q: %w", recipe.ResourceID, err)
	}
	// TODO check if anything else is needed to clean up Terraform installation.
	defer func() {
		err = installer.Remove(ctx)
	}()

	// Create working directory for Terraform execution
	workingDir := filepath.Join(workingDirRoot, util.NormalizeStringToLower(recipe.ResourceID), uuid.NewString())
	logger.Info(fmt.Sprintf("Creating Terraform working directory: %q", workingDir))
	if err = os.Mkdir(workingDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create working directory for terraform execution for Radius resource %q. %w", recipe.ResourceID, err)
	}
	defer os.RemoveAll(workingDir)

	// Generate Terraform json config in the working directory
	if err = d.generateJsonConfig(ctx, workingDir, recipe.Name, definition.TemplatePath); err != nil {
		return
	}

	if err = d.initAndApply(ctx, recipe.ResourceID, workingDir, execPath); err != nil {
		return
	}

	return
}

// Runs Terraform init and apply in the provided working directory.
func (d *terraformDriver) initAndApply(ctx context.Context, resourceID, workingDir, execPath string) error {
	tf, err := tfexec.NewTerraform(workingDir, execPath)
	if err != nil {
		return err
	}

	// Initialize Terraform
	if err := tf.Init(ctx); err != nil {
		return fmt.Errorf("terraform init failure. Radius resource %q. %w", resourceID, err)
	}

	// Apply Terraform configuration
	if err := tf.Apply(ctx); err != nil {
		return fmt.Errorf("terraform apply failure. Radius resource %q. %w", resourceID, err)
	}

	return nil
}

// Generate Terraform configuration in JSON format for required providers and modules
// and write it to a file in the specified working directory.
// This JSON configuration is needed to initialize and apply Terraform modules.
// See https://www.terraform.io/docs/language/syntax/json.html for more information
// on the JSON syntax for Terraform configuration.
// templatePath is the path to the Terraform module source, e.g. "Azure/cosmosdb/azurerm".
func (d *terraformDriver) generateJsonConfig(ctx context.Context, workingDir, recipeName, templatePath string) error {
	// TODO hardcoding provider data until we implement a way to pass this in.
	tfConfig := TerraformConfig{
		Terraform: TerraformDefinition{
			RequiredProviders: map[string]interface{}{
				"azurerm": map[string]interface{}{
					"source":  "hashicorp/azurerm",
					"version": "~> 3.0.2",
				},
			},
			RequiredVersion: ">= 1.1.0",
		},
		Provider: map[string]interface{}{
			"azurerm": map[string]interface{}{
				"features": map[string]interface{}{},
			},
		},
		Module: map[string]ModuleData{
			recipeName: {
				Source:  templatePath,
				Version: "1.0.0", // TODO determine how to pass this in.
			},
		},
	}

	// Convert the Terraform config to JSON
	jsonData, err := json.MarshalIndent(tfConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("Error marshalling JSON: %w", err)
	}

	// Write the JSON data to a file in the working directory
	configFilePath := fmt.Sprintf("%s/main.json", workingDir)
	file, err := os.Create(configFilePath)
	if err != nil {
		return fmt.Errorf("Error creating file: %w", err)
	}
	defer file.Close()

	_, err = file.Write(jsonData)
	if err != nil {
		return fmt.Errorf("Error writing to file: %w", err)
	}

	return nil
}
