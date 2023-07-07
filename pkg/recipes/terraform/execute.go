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
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	install "github.com/hashicorp/hc-install"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/terraform/config"
	"github.com/project-radius/radius/pkg/sdk"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

// NewExecutor creates a new Executor to execute a Terraform recipe.
func NewExecutor(ucpConn *sdk.Connection) *executor {
	return &executor{ucpConn: ucpConn}
}

const (
	executionSubDir = "deploy"
)

var _ TerraformExecutor = (*executor)(nil)

type executor struct {
	// ucpConn represents the configuration needed to connect to UCP, required to fetch cloud provider credentials.
	ucpConn *sdk.Connection
}

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

	// Generate Terraform json config in the working directory
	err = config.GenerateMainConfigFile(ctx, options.EnvRecipe, options.ResourceRecipe, workingDir)
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
	logger := logr.FromContextOrDiscard(ctx)

	workingDir := filepath.Join(tfDir, executionSubDir)
	logger.Info(fmt.Sprintf("Creating Terraform working directory: %q", workingDir))
	if err := os.MkdirAll(workingDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create working directory for terraform execution: %w", err)
	}

	return workingDir, nil
}

// Runs Terraform init and apply in the provided working directory.
func initAndApply(ctx context.Context, workingDir, execPath string) error {
	logger := logr.FromContextOrDiscard(ctx)

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
