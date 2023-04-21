// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package show

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
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

The recipe show command outputs details about a recipe. This includes the name, resource type, parameters, parameter details and template path.
	
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
		Args: cobra.ExactArgs(1),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad recipe show` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface
	Workspace         *workspaces.Workspace
	RecipeName        string
	Format            string
}

// NewRunner creates a new instance of the `rad recipe show` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		ConnectionFactory: factory.GetConnectionFactory(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad recipe show` command.
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

	recipeName, err := cli.RequireRecipeNameArgs(cmd, args)
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

// Run runs the `rad recipe show` command.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	recipeDetails, err := client.ShowRecipe(ctx, r.Workspace.Environment, r.RecipeName)
	if err != nil {
		return err
	}
	recipe := EnvironmentRecipe{
		RecipeName:   r.RecipeName,
		LinkType:     *recipeDetails.LinkType,
		TemplatePath: *recipeDetails.TemplatePath,
	}

	err = r.Output.WriteFormatted(r.Format, recipe, objectformats.GetRecipeTableFormat())
	if err != nil {
		return err
	}

	fmt.Println("")

	var recipeParams []EnvironmentRecipe

	for parameter := range recipeDetails.Parameters {
		values := recipeDetails.Parameters[parameter].(map[string]any)

		var paramItem EnvironmentRecipe
		paramItem.ParameterName = parameter
		paramItem.ParameterDefaultValue = "-"
		paramItem.ParameterMaxValue = "-"
		paramItem.ParameterMinValue = "-"
		for paramDetailName, paramDetailValue := range values {
			if paramDetailValue == nil {
				continue
			}

			switch paramDetailName {
			case "type":
				paramItem.ParameterType = paramDetailValue.(string)
			case "defaultValue":
				paramItem.ParameterDefaultValue = paramDetailValue
			case "maxValue":
				paramItem.ParameterMaxValue = fmt.Sprintf("%v", paramDetailValue.(float64))
			case "minValue":
				paramItem.ParameterMinValue = fmt.Sprintf("%v", paramDetailValue.(float64))
			}
		}

		recipeParams = append(recipeParams, paramItem)
	}

	err = r.Output.WriteFormatted(r.Format, recipeParams, objectformats.GetRecipeParamsTableFormat())
	if err != nil {
		return err
	}

	if len(recipeParams) == 0 {
		fmt.Println("(No parameters available)")
	}

	return nil
}

type EnvironmentRecipe struct {
	RecipeName            string      `json:"recipeName,omitempty"`
	LinkType              string      `json:"linkType,omitempty"`
	TemplatePath          string      `json:"templatePath,omitempty"`
	ParameterName         string      `json:"parameterName,omitempty"`
	ParameterDefaultValue interface{} `json:"parameterDefaultValue,omitempty"`
	ParameterType         string      `json:"parameterType,omitempty"`
	ParameterMaxValue     string      `json:"parameterMaxValue,omitempty"`
	ParameterMinValue     string      `json:"parameterMinValue,omitempty"`
}
