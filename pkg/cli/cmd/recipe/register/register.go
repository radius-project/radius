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

package register

import (
	"context"
	"fmt"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/bicep"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad recipe register` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "register [recipe-name]",
		Short: "Add a recipe to an environment.",
		Long: `Add a recipe to an environment.
You can specify parameters using the '--parameter' flag ('-p' for short). Parameters can be passed as:
		
- A file containing a single value in JSON format
- A key-value-pair passed in the command line
		`,
		Example: `
# Add a recipe to an environment
rad recipe register cosmosdb -e env_name -w workspace --template-kind bicep --template-path template_path --link-type Applications.Link/mongoDatabases
		
# Specify a parameter
rad recipe register cosmosdb -e env_name -w workspace --template-kind bicep --template-path template_path --link-type Applications.Link/mongoDatabases --parameters throughput=400
		
# specify multiple parameters using a JSON parameter file
rad recipe register cosmosdb -e env_name -w workspace --template-kind bicep --template-path template_path --link-type Applications.Link/mongoDatabases --parameters @myfile.json
		`,
		Args: cobra.ExactArgs(1),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	cmd.Flags().String("template-kind", "", "specify the kind for the template provided by the recipe.")
	_ = cmd.MarkFlagRequired("template-kind")
	cmd.Flags().String("template-path", "", "specify the path to the template provided by the recipe.")
	_ = cmd.MarkFlagRequired("template-path")
	cmd.Flags().String("link-type", "", "specify the type of the link this recipe can be consumed by")
	_ = cmd.MarkFlagRequired("link-type")
	commonflags.AddParameterFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad recipe register` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface
	Workspace         *workspaces.Workspace
	TemplateKind      string
	TemplatePath      string
	LinkType          string
	RecipeName        string
	Parameters        map[string]map[string]any
}

// NewRunner creates a new instance of the `rad recipe register` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		ConnectionFactory: factory.GetConnectionFactory(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad recipe register` command.
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

	templateKind, templatePath, err := requireRecipeProperties(cmd)
	if err != nil {
		return err
	}
	r.TemplateKind = templateKind
	r.TemplatePath = templatePath

	linkType, err := cli.RequireLinkType(cmd)
	if err != nil {
		return err
	}
	r.LinkType = linkType

	recipeName, err := cli.RequireRecipeNameArgs(cmd, args)
	if err != nil {
		return err
	}
	r.RecipeName = recipeName

	parameterArgs, err := cmd.Flags().GetStringArray("parameters")
	if err != nil {
		return err
	}

	parser := bicep.ParameterParser{FileSystem: bicep.OSFileSystem{}}
	r.Parameters, err = parser.Parse(parameterArgs...)
	if err != nil {
		return err
	}

	return nil
}

// Run runs the `rad recipe register` command.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	envResource, err := client.GetEnvDetails(ctx, r.Workspace.Environment)
	if err != nil {
		return err
	}

	envRecipes := envResource.Properties.Recipes
	if envRecipes == nil {
		envRecipes = map[string]map[string]*corerp.EnvironmentRecipeProperties{}
	}

	properties := &corerp.EnvironmentRecipeProperties{
		TemplateKind: &r.TemplateKind,
		TemplatePath: &r.TemplatePath,
		Parameters:   bicep.ConvertToMapStringInterface(r.Parameters),
	}
	if val, ok := envRecipes[r.LinkType]; ok {
		val[r.RecipeName] = properties
	} else {
		envRecipes[r.LinkType] = map[string]*corerp.EnvironmentRecipeProperties{
			r.RecipeName: properties,
		}
	}
	envResource.Properties.Recipes = envRecipes

	err = client.CreateEnvironment(ctx, r.Workspace.Environment, v1.LocationGlobal, envResource.Properties)
	if err != nil {
		return &cli.FriendlyError{Message: fmt.Sprintf("failed to register the recipe %s to the environment %s: %s", r.RecipeName, *envResource.ID, err.Error())}
	}

	r.Output.LogInfo("Successfully linked recipe %q to environment %q ", r.RecipeName, r.Workspace.Environment)
	return nil
}

func requireRecipeProperties(cmd *cobra.Command) (templateKind, templatePath string, err error) {
	templateKind, err = cmd.Flags().GetString("template-kind")
	if err != nil {
		return "", "", err
	}

	templatePath, err = cmd.Flags().GetString("template-path")
	if err != nil {
		return "", "", err
	}

	return templateKind, templatePath, nil
}
