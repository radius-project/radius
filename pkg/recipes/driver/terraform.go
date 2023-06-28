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

package driver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/terraform"
	"github.com/project-radius/radius/pkg/sdk"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ Driver = (*terraformDriver)(nil)

// NewTerraformDriver creates a new instance of driver to execute a Terraform recipe.
func NewTerraformDriver(ucpConn sdk.Connection, options TerraformOptions) Driver {
	return &terraformDriver{terraformExecutor: terraform.NewExecutor(&ucpConn), options: options}
}

// Options represents the options required for execution of Terraform driver.
type TerraformOptions struct {
	// Path is the path to the directory mounted to the container where terraform can be installed and executed.
	Path string

	// OperationID is a unique id of the operation that the Recipe is being executed by.
	// OperationID string
}

// terraformDriver represents a driver to interact with Terraform Recipe - deploy recipe, delete resources, etc.
type terraformDriver struct {
	// terraformExecutor is used to execute Terraform commands - deploy, destroy, etc.
	terraformExecutor terraform.TerraformExecutor

	// options contains options required to execute a Terraform recipe, such as the path to the directory mounted to the container where Terraform can be executed in sub directories.
	options TerraformOptions
}

// Execute deploys a Terraform recipe by using the Terraform CLI through terraform-exec
func (d *terraformDriver) Execute(ctx context.Context, configuration recipes.Configuration, recipe recipes.ResourceMetadata, definition recipes.EnvironmentDefinition) (*recipes.RecipeOutput, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	if d.options.Path == "" {
		return nil, errors.New("path is a required option for Terraform driver")
	}

	logger.Info(fmt.Sprintf("Deploying recipe: %q, template: %q", recipe.Name, definition.TemplatePath))
	// We need a unique directory per execution of terraform. We generate this using the unique operation id of the async request so that names are always unique,
	// but we can also trace them to the resource we were working on through operationID.
	operationID := v1.ARMRequestContextFromContext(ctx).OperationID
	dirID := ""
	if operationID != uuid.Nil {
		dirID = operationID.String()
	} else {
		// If the operationID is nil, we generate a new UUID a unique directory name. Ideally operationID should not be nil.
		logger.Info("Empty operation ID provided in the request context, using uuid to generate a unique directory name")
		dirID = uuid.NewString()
	}
	requestDirPath := filepath.Join(d.options.Path, dirID)
	if err := os.MkdirAll(requestDirPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory %q to execute terraform: %w", requestDirPath, err)
	}
	defer func() {
		if err := os.RemoveAll(requestDirPath); err != nil {
			logger.Info(fmt.Sprintf("Failed to cleanup Terraform execution directory %q. Err: %s", requestDirPath, err.Error()))
		}
	}()

	recipeOutputs, err := d.terraformExecutor.Deploy(ctx, terraform.Options{
		RootDir:        requestDirPath,
		EnvConfig:      &configuration,
		ResourceRecipe: &recipe,
		EnvRecipe:      &definition,
	})
	if err != nil {
		return nil, err
	}

	return recipeOutputs, nil
}
