// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package show

import (
	"context"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad recipe list` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:     "show",
		Short:   "Show link recipe details",
		Long:    "Show link recipe parameters within an environment",
		Example: `rad recipe show <recipe-name>`,
		RunE:    framework.RunCommand(runner),
		Args:    cobra.ExactArgs(0),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	commonflags.AddRecipeFlag(cmd)
	_ = cmd.MarkFlagRequired("name")

	return cmd, runner
}

// Runner is the runner implementation for the `rad recipe list` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface
	Workspace         *workspaces.Workspace
	RecipeName        string
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
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	// Validate command line args
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	// TODO: support fallback workspace
	if !r.Workspace.IsNamedWorkspace() {
		return workspaces.ErrNamedWorkspaceRequired
	}

	environment, err := cli.RequireEnvironmentName(cmd, args, *workspace)
	if err != nil {
		return err
	}
	r.Workspace.Environment = environment

	recipeName, err := cli.RequireRecipeName(cmd)
	if err != nil {
		return err
	}
	r.RecipeName = recipeName

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	return nil
}

// Run runs the `rad recipe list` command.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}
	recipeDetails, err := client.ShowRecipe(ctx, r.Workspace.Environment, r.RecipeName)
	if err != nil {
		return err
	}
	var recipeParams []EnvironmentRecipe
	var index = 0
	for paramName, param := range recipeDetails.Parameters {
		var recipe EnvironmentRecipe
		if index == 0 {
			recipe = EnvironmentRecipe{
				RecipeName:       r.RecipeName,
				LinkType:         *recipeDetails.LinkType,
				TemplatePath:     *recipeDetails.TemplatePath,
				ParameterName:    paramName,
				ParameterDetails: param,
			}
		} else {
			recipe = EnvironmentRecipe{
				ParameterName:    paramName,
				ParameterDetails: param,
			}
		}

		recipeParams = append(recipeParams, recipe)
		index += 1
	}
	err = r.Output.WriteFormatted(r.Format, recipeParams, objectformats.GetRecipeParamsTableFormats())
	if err != nil {
		return err
	}

	return nil
}

type EnvironmentRecipe struct {
	RecipeName       string      `json:"recipeName,omitempty"`
	LinkType         string      `json:"linkType,omitempty"`
	TemplatePath     string      `json:"templatePath,omitempty"`
	ParameterName    string      `json:"parameterName,omitempty"`
	ParameterDetails interface{} `json:"parameterDetails,omitempty"`
}
