// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package show

import (
	"context"
	"sort"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad recipe show` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "show [recipe-name]",
		Short: "Show recipe details",
		Long: `Show recipe details
	
	The recipe show command outputs details about a recipe. This includes the name, resource type, parameters, and template path.
	
	By default, the command is scoped to the resource group and environment defined in your rad.yaml workspace file. You can optionally override these values through the environment and group flags.
	
	By default, the command outputs a human-readable table. You can customize the output format with the output flag.`,
		Example: `
	# show the details of a recipe
	rad recipe show redis-prod
	
	# show the details of a recipe, with a JSON output
	rad recipe show redis-prod --output json
	
	# show the details of a recipe, with a specified environment and group
	rad recipe show redis-dev --group dev --environment dev`,
		RunE: framework.RunCommand(runner),
		Args: cobra.ExactArgs(0),
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
	if format == "" {
		format = "table"
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

	_, _, err = cmd.CheckIfRecipeExists(ctx, client, r.Workspace.Environment, r.RecipeName)
	if err != nil {
		return err
	}

	recipeDetails, err := client.ShowRecipe(ctx, r.Workspace.Environment, r.RecipeName)
	if err != nil {
		return err
	}

	var recipeParams []EnvironmentRecipe
	var index = 0
	keys := make([]string, 0, len(recipeDetails.Parameters))

	for k := range recipeDetails.Parameters {
		keys = append(keys, k)
	}

	// to keep order of parameters consistent - sort.
	sort.Strings(keys)
	for _, k := range keys {
		param := recipeDetails.Parameters[k]
		var recipe EnvironmentRecipe
		if index == 0 {
			recipe = EnvironmentRecipe{
				RecipeName:       r.RecipeName,
				LinkType:         *recipeDetails.LinkType,
				TemplatePath:     *recipeDetails.TemplatePath,
				ParameterName:    k,
				ParameterDetails: param,
			}
		} else {
			recipe = EnvironmentRecipe{
				ParameterName:    k,
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
