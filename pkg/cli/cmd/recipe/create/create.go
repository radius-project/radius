// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package create

import (
	"context"
	"errors"
	"fmt"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	corerpapps "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/spf13/cobra"
)

func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:     "create",
		Short:   "Add a link recipe to an environment.",
		Long:    `Add a link recipe to an environment.`,
		Example: `rad recipe create --name cosmosdb -e env_name -w workspace --template-path template_path --link-type Applications.Link/mongoDatabases`,
		Args:    cobra.ExactArgs(0),
		RunE:    framework.RunCommand(runner),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	cmd.Flags().String("template-path", "", "specify the path to the template provided by the recipe.")
	_ = cmd.MarkFlagRequired("template-path")
	cmd.Flags().String("link-type", "", "specify the type of the link this recipe can be consumed by")
	_ = cmd.MarkFlagRequired("link-type")
	cmd.Flags().String("name", "", "specify the name of the recipe")
	_ = cmd.MarkFlagRequired("name")

	return cmd, runner
}

type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface
	Workspace         *workspaces.Workspace
	TemplatePath      string
	LinkType          string
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

	templatePath, err := requireTemplatePath(cmd)
	if err != nil {
		return err
	}
	r.TemplatePath = templatePath

	linkType, err := requireLinkType(cmd)
	if err != nil {
		return err
	}
	r.LinkType = linkType

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
	if recipeProperties[r.RecipeName] != nil {
		return &cli.FriendlyError{Message: fmt.Sprintf("recipe with name %q alredy exists in the environment %q", r.RecipeName, r.Workspace.Environment)}
	}
	if recipeProperties != nil {
		recipeProperties[r.RecipeName] = &corerpapps.EnvironmentRecipeProperties{
			LinkType:     &r.LinkType,
			TemplatePath: &r.TemplatePath,
		}
	} else {
		recipeProperties = map[string]*corerpapps.EnvironmentRecipeProperties{
			r.RecipeName: {
				LinkType:     &r.LinkType,
				TemplatePath: &r.TemplatePath,
			},
		}
	}
	namespace := cmd.GetNamespace(envResource)

	isEnvCreated, err := client.CreateEnvironment(ctx, r.Workspace.Environment, v1.LocationGlobal, namespace, "Kubernetes", *envResource.ID, recipeProperties, envResource.Properties.Providers, *envResource.Properties.UseDevRecipes)
	if err != nil || !isEnvCreated {
		return &cli.FriendlyError{Message: fmt.Sprintf("failed to update Applications.Core/environments resource %s with recipe: %s", *envResource.ID, err.Error())}
	}

	r.Output.LogInfo("Successfully linked recipe %q to environment %q ", r.RecipeName, r.Workspace.Environment)
	return nil
}

func requireTemplatePath(cmd *cobra.Command) (string, error) {
	templatePath, err := cmd.Flags().GetString("template-path")
	if err != nil {
		return templatePath, err
	}
	return templatePath, nil

}

func requireLinkType(cmd *cobra.Command) (string, error) {
	linkType, err := cmd.Flags().GetString("link-type")
	if err != nil {
		return linkType, err
	}
	return linkType, nil
}

func requireRecipeName(cmd *cobra.Command) (string, error) {
	recipeName, err := cmd.Flags().GetString("name")
	if recipeName == "" {
		return "", errors.New("recipe name cannot be empty")
	}
	if err != nil {
		return recipeName, err
	}
	return recipeName, nil
}
