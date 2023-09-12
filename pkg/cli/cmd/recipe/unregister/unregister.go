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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad recipe unregister` command.
//

// NewCommand creates a new cobra command for unregistering a recipe from an environment, which takes in a factory and returns a cobra command
// and a runner. It also sets up flags for output, workspace, resource group, environment name and portable resource type, with resource-type being a required flag.
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
	commonflags.AddResourceTypeFlag(cmd)
	_ = cmd.MarkFlagRequired(cli.ResourceTypeFlag)

	return cmd, runner
}

// Runner is the runner implementation for the `rad recipe unregister` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface
	Workspace         *workspaces.Workspace
	RecipeName        string
	ResourceType      string
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
//

// // Runner.Validate checks the command line arguments for a workspace, environment, recipe name, and resource type, and
// returns an error if any of these are not present.
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

	resourceType, err := cli.GetResourceType(cmd)
	if err != nil {
		return err
	}
	r.ResourceType = resourceType

	return nil
}

// Run runs the `rad recipe unregister` command.
//

// Run checks if a recipe exists in an environment, deletes the recipe from the environment's properties, and then
// creates the environment with the updated properties. It returns an error if any of these steps fail.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	envResource, recipeProperties, err := cmd.CheckIfRecipeExists(ctx, client, r.Workspace.Environment, r.RecipeName, r.ResourceType)
	if err != nil {
		return err
	}
	if val, ok := recipeProperties[r.ResourceType]; ok {
		delete(val, r.RecipeName)
		if len(val) == 0 {
			delete(recipeProperties, r.ResourceType)
		}
	}
	envResource.Properties.Recipes = recipeProperties
	err = client.CreateEnvironment(ctx, r.Workspace.Environment, v1.LocationGlobal, envResource.Properties)
	if err != nil {
		return clierrors.MessageWithCause(err, "Failed to unregister the recipe %s from the environment %s.", r.RecipeName, *envResource.ID)
	}

	r.Output.LogInfo("Successfully unregistered recipe %q from environment %q ", r.RecipeName, r.Workspace.Environment)
	return nil
}
