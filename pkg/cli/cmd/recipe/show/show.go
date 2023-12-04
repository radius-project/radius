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

package show

import (
	"context"
	"fmt"
	"sort"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	types "github.com/radius-project/radius/pkg/cli/cmd/recipe"
	"github.com/radius-project/radius/pkg/cli/cmd/recipe/common"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad recipe show` command.
//

// NewCommand creates a new cobra command that can be used to show recipe details, such as the name, resource type,
// parameters, parameter details and template path, with the option to customize the output format.
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
rad recipe show redis-prod --resource-type Applications.Datastores/redisCaches

# show the details of a recipe, with a JSON output
rad recipe show redis-prod --resource-type Applications.Datastores/redisCaches --output json
	
# show the details of a recipe, with a specified environment and group
rad recipe show redis-dev --resource-type Applications.Datastores/redisCaches --group dev --environment dev`,
		RunE: framework.RunCommand(runner),
		Args: cobra.ExactArgs(1),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	commonflags.AddResourceTypeFlag(cmd)
	_ = cmd.MarkFlagRequired(cli.ResourceTypeFlag)

	return cmd, runner
}

// Runner is the runner implementation for the `rad recipe show` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface
	Workspace         *workspaces.Workspace
	RecipeName        string
	ResourceType      string
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
//

// Validate takes in a command and a slice of strings and validates the command line arguments, setting the workspace, environment,
// recipe name, portable resource type and output format in the Runner struct. It returns an error if any of the arguments are invalid.
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

	resourceType, err := cli.GetResourceType(cmd)
	if err != nil {
		return err
	}
	r.ResourceType = resourceType

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
//

// Run retrieves the recipe details and parameters from the Applications Management service and prints them in the
// specified format. It returns an error if one occurs.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	recipeDetails, err := client.ShowRecipe(ctx, r.Workspace.Environment, v20231001preview.RecipeGetMetadata{Name: &r.RecipeName, ResourceType: &r.ResourceType})
	if err != nil {
		return err
	}

	recipe := types.EnvironmentRecipe{
		Name:         r.RecipeName,
		ResourceType: r.ResourceType,
		TemplatePath: *recipeDetails.TemplatePath,
		TemplateKind: *recipeDetails.TemplateKind,
	}
	if recipeDetails.TemplateVersion != nil {
		recipe.TemplateVersion = *recipeDetails.TemplateVersion
	}

	if recipeDetails.PlainHTTP != nil {
		recipe.PlainHTTP = *recipeDetails.PlainHTTP
	}

	err = r.Output.WriteFormatted(r.Format, recipe, common.RecipeFormat())
	if err != nil {
		return err
	}

	r.Output.LogInfo("")

	var recipeParams []types.RecipeParameter

	for parameter := range recipeDetails.Parameters {
		values := recipeDetails.Parameters[parameter].(map[string]any)

		paramItem := types.RecipeParameter{
			Name:         parameter,
			DefaultValue: "-",
			MaxValue:     "-",
			MinValue:     "-",
		}

		for paramDetailName, paramDetailValue := range values {
			switch paramDetailName {
			case "type":
				paramItem.Type = paramDetailValue.(string)
			case "defaultValue":
				paramItem.DefaultValue = paramDetailValue
			case "maxValue":
				paramItem.MaxValue = fmt.Sprintf("%v", paramDetailValue.(float64))
			case "minValue":
				paramItem.MinValue = fmt.Sprintf("%v", paramDetailValue.(float64))
			}
		}

		recipeParams = append(recipeParams, paramItem)
	}

	// Sort parameters so that results are deterministic.
	sort.Slice(recipeParams, func(i, j int) bool {
		return recipeParams[i].Name > recipeParams[j].Name
	})

	err = r.Output.WriteFormatted(r.Format, recipeParams, common.RecipeParametersFormat())
	if err != nil {
		return err
	}

	if len(recipeParams) == 0 {
		r.Output.LogInfo("No parameters available")
	}

	return nil
}
