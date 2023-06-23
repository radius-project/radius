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
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/terraform"
	"github.com/project-radius/radius/pkg/sdk"
	"github.com/project-radius/radius/pkg/ucp/util"
)

var _ Driver = (*terraformDriver)(nil)

// NewTerraformDriver creates a new instance of driver to execute a Terraform recipe.
func NewTerraformDriver(ucpConn sdk.Connection, directoryPath string) Driver {
	tfExecutor := terraform.NewExecutor(&ucpConn)
	return &terraformDriver{terraformExecutor: tfExecutor, ucpConn: ucpConn, directoryPath: directoryPath}
}

type terraformDriver struct {
	terraformExecutor terraform.TerraformExecutor
	ucpConn           sdk.Connection
	directoryPath     string // DirectoryPath is the path to the directory mounted to the container where Terraform will be installed and executed (module deployment), in sub directories.
}

// Execute deploys a Terraform recipe by using the Terraform CLI through terraform-exec
func (d *terraformDriver) Execute(ctx context.Context, configuration recipes.Configuration, recipe recipes.ResourceMetadata, definition recipes.EnvironmentDefinition) (*recipes.RecipeOutput, error) {
	logger := logr.FromContextOrDiscard(ctx)

	logger.Info(fmt.Sprintf("Deploying recipe: %q, template: %q", recipe.Name, definition.TemplatePath))
	resourceDirPath := d.directoryPath + "/" + util.NormalizeStringToLower(recipe.ResourceID) + "-" + uuid.NewString()

	recipeOutputs, err := d.terraformExecutor.Deploy(ctx, terraform.TerraformOptions{
		RootDir:        resourceDirPath,
		EnvConfig:      &configuration,
		ResourceRecipe: &recipe,
		EnvRecipe:      &definition,
	})
	if err != nil {
		cleanup(ctx, resourceDirPath)
		return nil, err
	}

	// Cleanup Terraform directories
	cleanup(ctx, resourceDirPath)

	return recipeOutputs, nil
}

func cleanup(ctx context.Context, tfDir string) {
	logger := logr.FromContextOrDiscard(ctx)

	err := os.RemoveAll(tfDir)
	if err != nil {
		logger.Error(err, "Failed to remove Terraform installation directory")
	}
}
