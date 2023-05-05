// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package driver

import (
	"context"
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

func (d *terraformDriver) Execute(ctx context.Context, configuration recipes.Configuration, recipe recipes.Metadata, definition recipes.Definition) (*recipes.RecipeOutput, error) {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("Deploying recipe: %q, template: %q", recipe.Name, definition.TemplatePath))

	// Create Terraform installation directory
	installDir := filepath.Join(installDirRoot, util.NormalizeStringToLower(recipe.ResourceID), uuid.NewString())
	logger.Info(fmt.Sprintf("Creating Terraform install directory: %q", installDir))
	if err := os.MkdirAll(installDir, os.ModePerm); err != nil {
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
	defer installer.Remove(ctx)

	// Create working directory for Terraform execution
	workingDir := filepath.Join(workingDirRoot, util.NormalizeStringToLower(recipe.ResourceID), uuid.NewString())
	logger.Info(fmt.Sprintf("Creating Terraform working directory: %q", workingDir))
	if err := os.Mkdir(workingDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create working directory for terraform execution for Radius resource %q. %w", recipe.ResourceID, err)
	}
	defer os.RemoveAll(workingDir)

	if err := d.initAndApply(ctx, recipe.ResourceID, workingDir, execPath); err != nil {
		return nil, err
	}

	return &recipes.RecipeOutput{}, nil
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
