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

package list

import (
	"context"
	"sort"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	types "github.com/radius-project/radius/pkg/cli/cmd/recipe"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/objectformats"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerp "github.com/radius-project/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad recipe list` command.
//

// NewCommand creates a new Cobra command and a Runner object, configures the command with flags and arguments, and
// returns the command and the runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List recipes",
		Long:    "List recipes within an environment",
		Example: `rad recipe list`,
		RunE:    framework.RunCommand(runner),
		Args:    cobra.ExactArgs(0),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad recipe list` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface
	Workspace         *workspaces.Workspace
	Format            string
}

// NewRunner creates a new instance of the `rad recipe list` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		ConnectionFactory: factory.GetConnectionFactory(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad recipe list` command.
//

// Validate checks the command line arguments for a workspace, environment name, and output format, and sets the corresponding
// fields in the Runner struct if they are valid. If any of the arguments are invalid, an error is returned.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	// Validate command line args
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	environment, err := cli.RequireEnvironmentName(cmd, args, *workspace)
	if err != nil {
		return err
	}
	r.Workspace.Environment = environment

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	return nil
}

// Run runs the `rad recipe list` command.
//

// Run retrieves environment recipes from the given workspace and writes them to the output in the specified format.
// It returns an error if the connection to the workspace fails or if there is an error writing to the output.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}
	envResource, err := client.GetEnvDetails(ctx, r.Workspace.Environment)
	if err != nil {
		return err
	}
	var envRecipes []types.EnvironmentRecipe
	for resourceType, recipes := range envResource.Properties.Recipes {
		for recipeName, recipeDetails := range recipes {
			recipe := types.EnvironmentRecipe{}
			switch c := recipeDetails.(type) {
			case *corerp.TerraformRecipeProperties:
				recipe = types.EnvironmentRecipe{
					Name:            recipeName,
					LinkType:        resourceType,
					TemplatePath:    *c.TemplatePath,
					TemplateKind:    *c.TemplateKind,
					TemplateVersion: *c.TemplateVersion,
				}
			case *corerp.BicepRecipeProperties:
				recipe = types.EnvironmentRecipe{
					Name:         recipeName,
					LinkType:     resourceType,
					TemplatePath: *c.TemplatePath,
					TemplateKind: *c.TemplateKind,
				}
			}
			envRecipes = append(envRecipes, recipe)
		}
	}
	sort.Slice(envRecipes, func(i, j int) bool {
		return envRecipes[i].Name < envRecipes[j].Name
	})
	err = r.Output.WriteFormatted(r.Format, envRecipes, objectformats.GetEnvironmentRecipesTableFormat())
	if err != nil {
		return err
	}

	return nil
}
