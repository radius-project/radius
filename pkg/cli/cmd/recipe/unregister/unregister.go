// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
		Use:     "unregister",
		Short:   "Unregister a link recipe from an environment",
		Long:    `Unregister a link recipe from an environment`,
		Example: `rad recipe unregister --name cosmosdb`,
		Args:    cobra.ExactArgs(0),
		RunE:    framework.RunCommand(runner),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	cmd.Flags().String("name", "", "specify the name of the recipe")
	_ = cmd.MarkFlagRequired("name")

	return cmd, runner
}

// Runner is the runner implementation for the `rad recipe unregister` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface
	Workspace         *workspaces.Workspace
	RecipeName        string
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

	// TODO: support fallback workspace
	if !r.Workspace.IsNamedWorkspace() {
		return workspaces.ErrNamedWorkspaceRequired
	}

	environment, err := cli.RequireEnvironmentName(cmd, args, *workspace)
	if err != nil {
		return err
	}
	r.Workspace.Environment = environment

	recipeName, err := requireRecipeName(cmd)
	if err != nil {
		return err
	}
	r.RecipeName = recipeName
	return nil
}

// Run runs the `rad recipe unregister` command.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}
	envResource, err := client.GetEnvDetails(ctx, r.Workspace.Environment)
	if err != nil {
		return err
	}

	recipeProperties := envResource.Properties.Recipes

	if recipeProperties[r.RecipeName] == nil {
		return &cli.FriendlyError{Message: fmt.Sprintf("recipe %q is not part of the environment %q ", r.RecipeName, r.Workspace.Environment)}
	}
	namespace := cmd.GetNamespace(envResource)
	delete(recipeProperties, r.RecipeName)
	isEnvCreated, err := client.CreateEnvironment(ctx, r.Workspace.Environment, v1.LocationGlobal, namespace, "Kubernetes", *envResource.ID, recipeProperties, envResource.Properties.Providers, *envResource.Properties.UseDevRecipes)
	if err != nil || !isEnvCreated {
		return &cli.FriendlyError{Message: fmt.Sprintf("failed to unregister the recipe %s from the environment %s: %s", r.RecipeName, *envResource.ID, err.Error())}
	}

	r.Output.LogInfo("Successfully unregistered recipe %q from environment %q ", r.RecipeName, r.Workspace.Environment)
	return nil
}

func requireRecipeName(cmd *cobra.Command) (string, error) {
	recipeName, err := cmd.Flags().GetString("name")
	if err != nil {
		return recipeName, err
	}
	return recipeName, nil
}
