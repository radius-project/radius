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

package unregister

import (
	"context"
	"fmt"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad recipe unregister` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:     "unregister [recipe-name]",
		Short:   "Unregister a recipe from an environment",
		Long:    `Unregister a recipe from an environment`,
		Example: `rad recipe unregister cosmosdb`,
		Args:    cobra.ExactArgs(1),
		RunE:    framework.RunCommand(runner),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	commonflags.AddLinkTypeFlag(cmd)
	_ = cmd.MarkFlagRequired(cli.LinkTypeFlag)

	return cmd, runner
}

// Runner is the runner implementation for the `rad recipe unregister` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface
	Workspace         *workspaces.Workspace
	RecipeName        string
	LinkType          string
}

// NewRunner creates a new instance of the `rad recipe unregister` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		ConnectionFactory: factory.GetConnectionFactory(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad recipe unregister` command.
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

	recipeName, err := cli.RequireRecipeNameArgs(cmd, args)
	if err != nil {
		return err
	}
	r.RecipeName = recipeName

	linkType, err := cli.RequireLinkType(cmd)
	if err != nil {
		return err
	}
	r.LinkType = linkType

	return nil
}

// Run runs the `rad recipe unregister` command.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	envResource, recipeProperties, err := cmd.CheckIfRecipeExists(ctx, client, r.Workspace.Environment, r.RecipeName, r.LinkType)
	if err != nil {
		return err
	}
	if val, ok := recipeProperties[r.LinkType]; ok {
		delete(val, r.RecipeName)
		if len(val) == 0 {
			delete(recipeProperties, r.LinkType)
		}
	}
	envResource.Properties.Recipes = recipeProperties
	isEnvCreated, err := client.CreateEnvironment(ctx, r.Workspace.Environment, v1.LocationGlobal, envResource.Properties)
	if err != nil || !isEnvCreated {
		return &cli.FriendlyError{Message: fmt.Sprintf("failed to unregister the recipe %s from the environment %s: %s", r.RecipeName, *envResource.ID, err.Error())}
	}

	r.Output.LogInfo("Successfully unregistered recipe %q from environment %q ", r.RecipeName, r.Workspace.Environment)
	return nil
}
