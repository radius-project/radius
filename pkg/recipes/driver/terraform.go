// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package driver

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/releases"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/project-radius/radius/pkg/recipes"
)

var _ Driver = (*terraformDriver)(nil)

// NewTerraformDriver creates a new instance of Driver to execute a Terraform recipe.
func NewTerraformDriver() Driver {
	return &terraformDriver{}
}

type terraformDriver struct {
}

func (d *terraformDriver) Execute(ctx context.Context, configuration recipes.Configuration, recipe recipes.Metadata, definition recipes.Definition) (*recipes.RecipeOutput, error) {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("Deploying recipe: %q, template: %q", recipe.Name, definition.TemplatePath))

	// Generate Terraform configuration

	// Terraform init
	installer := &releases.ExactVersion{
		Product: product.Terraform,
		Version: version.Must(version.NewVersion("1.0.6")),
	}

	_, err := installer.Install(context.Background())
	if err != nil {
		return nil, err
	}
	_, err = tfexec.NewTerraform("", "")
	if err != nil {
		return nil, err
	}

	// Terraform import to load resource group
	// Get context parameter
	// Terraform apply with recipe input params and context param
	// Terraform destroy

	return &recipes.RecipeOutput{}, nil
}
