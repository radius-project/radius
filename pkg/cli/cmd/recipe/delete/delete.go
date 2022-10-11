// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package delete

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:     "delete",
		Short:   "Delete a connector recipe from an environment",
		Long:    `Delete a connector recipe from an environment`,
		Example: `rad recipe delete --name cosmosdb`,
		Args:    cobra.ExactArgs(0),
		RunE:    framework.RunCommand(runner),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	cmd.Flags().String("name", "", "specify the name of the recipe")

	return cmd, runner
}

type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface
	Workspace         *workspaces.Workspace
	RecipeName        string
}

func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		ConnectionFactory: factory.GetConnectionFactory(),
		Output:            factory.GetOutput(),
	}
}

func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	// Validate command line args
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	environmentName, err := cli.RequireEnvironmentName(cmd, args, *workspace)
	if err != nil {
		return err
	}
	r.Workspace.Environment = environmentName

	recipeName, err := requireRecipeName(cmd)
	if err != nil {
		return err
	}
	r.RecipeName = recipeName
	return nil
}

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
		return fmt.Errorf("recipe %q is not part of the environment %q ", r.RecipeName, r.Workspace.Environment)
	}
	delete(recipeProperties, r.RecipeName)
	isEnvCreated, err := client.CreateEnvironment(ctx, r.Workspace.Environment, "global", "default", "Kubernetes", *envResource.ID, recipeProperties, envResource.Properties.Providers)
	if err != nil || !isEnvCreated {
		return &cli.FriendlyError{Message: fmt.Sprintf("failed to update Applications.Core/environments resource %s with recipe: %s", *envResource.ID, err.Error())}
	}

	r.Output.LogInfo("Successfully deleted recipe %q from environment %q ", r.RecipeName, r.Workspace.Environment)
	return nil
}

func requireRecipeName(cmd *cobra.Command) (string, error) {
	recipeName, err := cmd.Flags().GetString("name")
	if err != nil {
		return recipeName, err
	}
	return recipeName, nil
}
