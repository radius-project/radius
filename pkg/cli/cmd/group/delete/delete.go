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

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/cmd"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

const (
	// scopeLocal is the constant for local scope
	scopeLocal = "local"

	// Message templates
	msgResourceGroupDeleted  = "System.Resources/resourceGroups/%s deleted"
	msgResourceGroupNotFound = "System.Resources/resourceGroups/%s not found"
)

// NewCommand creates an instance of the command and runner for the `rad group delete` command.
//

// NewCommand creates a new cobra command for deleting a resource group, which takes in a workspace and resource group
//
//	name as arguments, and a confirmation flag, and returns a cobra command and a runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "delete resourcegroupname",
		Short: "Delete a resource group",
		Long: `Delete a resource group and all its resources.

The command will:
- Check if the resource group contains any deployed resources
- Show an appropriate confirmation prompt based on whether resources exist
- Delete all resources in the group (if any) before deleting the group itself

Resources are deleted in stages rather than all at once. Workloads are deleted first, then secrets
and secret stores, then applications, and finally environments along with the recipe packs and
settings they reference. Deleting a resource runs its recipe, which needs that configuration to
still exist, so the configuration is removed last.

If any resource fails to delete, the remaining resources in the same stage still finish and every
failure is reported. Later stages are skipped and the resource group is left in place. Re-running
the command is safe and will retry the resources that are left.

Use the --yes flag to skip confirmation prompts.`,
		Example: `rad group delete rgprod
rad group delete rgprod --yes`,
		Args: cobra.MaximumNArgs(1),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddConfirmationFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad group delete` command.
type Runner struct {
	ConfigHolder         *framework.ConfigHolder
	ConnectionFactory    connections.Factory
	Output               output.Interface
	InputPrompter        prompt.Interface
	Workspace            *workspaces.Workspace
	UCPResourceGroupName string
	Confirmation         bool
}

// NewRunner creates a new instance of the `rad group delete` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
		InputPrompter:     factory.GetPrompter(),
	}
}

// Validate runs validation for the `rad group delete` command.
//

// Validate checks if the required workspace, resource group and confirmation flag are present and sets them in
// the Runner struct if they are. It returns an error if any of these are not present.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config)
	if err != nil {
		return err
	}

	resourceGroup, err := cli.RequireUCPResourceGroup(cmd, args)
	if err != nil {
		return err
	}

	yes, err := cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}

	r.UCPResourceGroupName = resourceGroup
	r.Workspace = workspace
	r.Confirmation = yes

	return nil
}

// Run runs the `rad group delete` command.
//

// Run enumerates the resource group, confirms the deletion with the user, deletes the group's
// resources in dependency order and then deletes the group itself.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return fmt.Errorf("failed to create management client: %w", err)
	}

	// The group is enumerated once and the same set is used for both the confirmation prompt and
	// the deletion, so that the user cannot be asked about one set of resources and have a
	// different set deleted.
	resources, listErr := client.ListResourcesInResourceGroup(ctx, scopeLocal, r.UCPResourceGroupName)
	if listErr != nil {
		// A 404 means the group does not exist, which is reported as a no-op below rather than as
		// a failure. Any other error means the contents are unknown, and deleting a group whose
		// contents cannot be enumerated would silently orphan them.
		if !clients.Is404Error(listErr) {
			return fmt.Errorf("unable to verify resource group contents: %w", listErr)
		}

		r.Output.LogInfo(msgResourceGroupNotFound, r.UCPResourceGroupName)

		return nil
	}

	// Handle confirmation prompts when --yes is not provided
	if !r.Confirmation {
		confirmed, err := r.promptForConfirmation(len(resources) > 0)
		if err != nil {
			return err
		}

		if !confirmed {
			return nil
		}
	}

	// Resources are deleted in dependency order. Deleting them in one unordered wave lets a recipe
	// pack, settings resource or environment be deleted before the recipe-driven resources that
	// need it in order to tear themselves down, which leaves those resources undeletable.
	tiers := cmd.GroupResourcesByDeleteTier(resources)
	if err := cmd.DeleteResourcesInTiers(ctx, client, r.Output, tiers, false); err != nil {
		return fmt.Errorf("failed to delete resources in resource group %s: %w", r.UCPResourceGroupName, err)
	}

	deleted, err := client.DeleteResourceGroupRecord(ctx, scopeLocal, r.UCPResourceGroupName)
	if err != nil {
		return fmt.Errorf("failed to delete resource group %s: %w", r.UCPResourceGroupName, err)
	}

	if deleted {
		r.Output.LogInfo(msgResourceGroupDeleted, r.UCPResourceGroupName)
	} else {
		r.Output.LogInfo(msgResourceGroupNotFound, r.UCPResourceGroupName)
	}

	return nil
}

// promptForConfirmation handles the confirmation prompts based on resource state
func (r *Runner) promptForConfirmation(hasResources bool) (bool, error) {
	var promptMsg string

	// At this point, listErr is either nil or a 404 error (other errors are handled earlier)
	if hasResources {
		promptMsg = fmt.Sprintf("The resource group %s contains deployed resources. Are you sure you want to delete the resource group and its resources?",
			r.UCPResourceGroupName)
	} else {
		promptMsg = fmt.Sprintf("The resource group %s is empty. Are you sure you want to delete the resource group?",
			r.UCPResourceGroupName)
	}

	return prompt.YesOrNoPrompt(promptMsg, prompt.ConfirmNo, r.InputPrompter)
}
