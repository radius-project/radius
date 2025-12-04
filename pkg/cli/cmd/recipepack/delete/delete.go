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

package delete

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	utils "github.com/radius-project/radius/pkg/cli/cmd"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	deleteConfirmationMsg   = "Are you sure you want to delete recipe pack '%s'?"
	msgDeletingRecipePack   = "Deleting recipe pack %s...\n"
	msgRecipePackDeleted    = "Recipe pack %s deleted."
	msgRecipePackNotFound   = "Recipe pack %s does not exist or has already been deleted."
	msgRecipePackNotDeleted = "Recipe pack %q NOT deleted"
)

// NewCommand creates a new Cobra command for deleting a recipe pack.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete recipe pack",
		Long:  "Delete a recipe pack from the current workspace scope.",
		Args:  cobra.ExactArgs(1),
		Example: `
# Delete specified recipe pack
rad recipe-pack delete my-recipe-pack

# Delete recipe pack in a specified resource group
rad recipe-pack delete my-recipe-pack --group my-group

# Delete recipe pack and bypass confirmation prompt
rad recipe-pack delete my-recipe-pack --yes
`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddConfirmationFlag(cmd)

	return cmd, runner
}

// Runner executes the recipe pack delete command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Workspace         *workspaces.Workspace
	Output            output.Interface
	InputPrompter     prompt.Interface

	RecipePackName          string
	Confirm                 bool
	RadiusCoreClientFactory *corerpv20250801.ClientFactory
}

// NewRunner creates a new instance of the recipe pack delete runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		ConnectionFactory: factory.GetConnectionFactory(),
		Output:            factory.GetOutput(),
		InputPrompter:     factory.GetPrompter(),
	}
}

// Validate ensures command arguments and flags are correct before execution.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	scope, err := cli.RequireScope(cmd, *r.Workspace)
	if err != nil {
		return err
	}
	r.Workspace.Scope = scope

	recipePackName, err := cli.RequireRecipePackNameArgs(cmd, args)
	if err != nil {
		return err
	}
	r.RecipePackName = recipePackName

	r.Confirm, err = cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}

	return nil
}

// Run performs the delete operation for the recipe pack.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	if !r.Confirm {
		confirmed, err := prompt.YesOrNoPrompt(fmt.Sprintf(deleteConfirmationMsg, r.RecipePackName), prompt.ConfirmNo, r.InputPrompter)
		if err != nil {
			return err
		}

		if !confirmed {
			r.Output.LogInfo(msgRecipePackNotDeleted, r.RecipePackName)
			return nil
		}
	}

	r.Output.LogInfo(msgDeletingRecipePack, r.RecipePackName)

	recipePack, err := client.GetRecipePack(ctx, r.RecipePackName)
	if clients.Is404Error(err) {
		return clierrors.Message("The recipe pack %q was not found or has been deleted.", r.RecipePackName)
	} else if err != nil {
		return err
	}

	envs := recipePack.Properties.ReferencedBy

	for _, env := range envs {
		ID, err := resources.Parse(*env)
		if err != nil {
			return err
		}

		cd, err := utils.InitializeRadiusCoreClientFactory(ctx, r.Workspace, ID.RootScope())
		if err != nil {
			return err
		}

		envClient := cd.NewEnvironmentsClient()

		resp, err := envClient.Get(ctx, *env, &corerpv20250801.EnvironmentsClientGetOptions{})
		if clients.Is404Error(err) {
			continue
		} else if err != nil {
			return err
		}

		res := resp.EnvironmentResource
		res.SystemData = nil
		for i, rp := range res.Properties.RecipePacks {
			if *rp == *recipePack.ID {
				res.Properties.RecipePacks = append(res.Properties.RecipePacks[:i], res.Properties.RecipePacks[i+1:]...)
				break
			}
		}

		_, err = envClient.CreateOrUpdate(ctx, *env, res, &corerpv20250801.EnvironmentsClientCreateOrUpdateOptions{})
		if err != nil {
			return clierrors.MessageWithCause(err, "Failed to update environment %s.", *env)
		}
	}

	deleted, err := client.DeleteRecipePack(ctx, r.RecipePackName)
	if err != nil {
		if clients.Is404Error(err) {
			r.Output.LogInfo(msgRecipePackNotFound, r.RecipePackName)
			return nil
		}
		return fmt.Errorf("failed to delete resource group %s: %w", r.RecipePackName, err)
	}

	if deleted {
		r.Output.LogInfo(msgRecipePackDeleted, r.RecipePackName)
	} else {
		r.Output.LogInfo(msgRecipePackNotFound, r.RecipePackName)
	}

	return nil
}
