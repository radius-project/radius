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

package preview

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	generated "github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
)

const (
	msgEnvironmentDeletedPreview  = "Radius.Core/environments/%s deleted"
	msgEnvironmentNotFoundPreview = "Radius.Core/environments/%s not found"
	msgDeletingResources          = "Deleting %d resource(s) in environment %s...\n"
	msgDeletingApplications       = "Deleting %d application(s) in environment %s...\n"
	msgDeletingApplication        = "  Deleting application %s..."
	msgForceWarning               = "WARNING: Force deleting an environment. Resources in non-terminal states may leave orphaned external resources that require manual cleanup."
)

// NewCommand creates an instance of the command and runner for the `rad env delete --preview` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete environment",
		Long: `Delete environment. Deletes the user's default environment by default.

In preview mode, deleting an environment also deletes the applications in that environment
and the resources deployed into it.`,
		Args: cobra.MaximumNArgs(1),
		RunE: framework.RunCommand(runner),
		Example: `
# Delete current environment
rad env delete

# Delete current environment and bypass confirmation prompt
rad env delete --yes

# Delete specified environment
rad env delete my-env

# Delete specified environment in a specified resource group
rad env delete my-env --group my-env

# Delete a Radius.Core environment and everything deployed into it
rad env delete my-env --preview

# Delete a Radius.Core environment, forcing deletion of resources in a non-terminal state
rad env delete my-env --force --preview
`,
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	commonflags.AddConfirmationFlag(cmd)
	commonflags.AddForceFlag(cmd)
	commonflags.AddOutputFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the preview `rad env delete` command.
type Runner struct {
	ConfigHolder            *framework.ConfigHolder
	Output                  output.Interface
	InputPrompter           prompt.Interface
	ConnectionFactory       connections.Factory
	Workspace               *workspaces.Workspace
	RadiusCoreClientFactory *corerpv20250801.ClientFactory

	Confirm         bool
	Force           bool
	EnvironmentName string
}

// NewRunner creates a new instance of the preview delete runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
		InputPrompter:     factory.GetPrompter(),
		ConnectionFactory: factory.GetConnectionFactory(),
	}
}

// Validate runs validation for the preview delete command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	// Allow '--group' to override scope
	scope, err := cli.RequireScope(cmd, *r.Workspace)
	if err != nil {
		return err
	}
	r.Workspace.Scope = scope

	r.EnvironmentName, err = cli.RequireEnvironmentNameArgs(cmd, args, *workspace)
	if err != nil {
		return err
	}

	r.Confirm, err = cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}

	r.Force, err = cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}

	_, err = cli.RequireOutput(cmd) // we ignore format for preview delete
	if err != nil {
		return err
	}

	return nil
}

// Run executes the preview delete command logic.
//
// Deleting an environment cascades to everything deployed into it: the resources that reference
// the environment, the applications in the environment, and finally the environment itself. This
// matches the behavior of the legacy Applications.Core/environments delete path, so that deleting
// an environment does not leave orphaned resources behind.
func (r *Runner) Run(ctx context.Context) error {
	if r.RadiusCoreClientFactory == nil {
		factory, err := cmd.InitializeRadiusCoreClientFactory(ctx, r.Workspace)
		if err != nil {
			return err
		}
		r.RadiusCoreClientFactory = factory
	}

	envClient := r.RadiusCoreClientFactory.NewEnvironmentsClient()

	// Check that the environment exists before enumerating its contents, so that deleting an
	// already-deleted environment is a no-op rather than an error.
	_, err := envClient.Get(ctx, r.Workspace.Scope, r.EnvironmentName, &corerpv20250801.EnvironmentsClientGetOptions{})
	if clients.Is404Error(err) {
		r.Output.LogInfo(msgEnvironmentNotFoundPreview, r.EnvironmentName)
		return nil
	} else if err != nil {
		return err
	}

	managementClient, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	environmentID := cmd.PreviewEnvironmentID(r.Workspace.Scope, r.EnvironmentName)

	applications, err := cmd.ListPreviewApplicationsInEnvironment(ctx, r.RadiusCoreClientFactory.NewApplicationsClient(), r.Workspace, environmentID)
	if err != nil && !clients.Is404Error(err) {
		return err
	}

	// Resources are collected from two directions: those that reference the environment directly,
	// and those owned by an application in the environment. A resource usually carries both
	// properties, but neither query is guaranteed to be a superset of the other, so both are
	// merged and de-duplicated by resource ID.
	resourcesInEnvironment, err := managementClient.ListResourcesInEnvironment(ctx, environmentID)
	if err != nil && !clients.Is404Error(err) {
		return err
	}

	resourceLists := [][]generated.GenericResource{resourcesInEnvironment}
	for _, application := range applications {
		if application.ID == nil {
			continue
		}

		resourcesInApplication, err := managementClient.ListResourcesInApplication(ctx, *application.ID)
		if err != nil && !clients.Is404Error(err) {
			return err
		}

		resourceLists = append(resourceLists, resourcesInApplication)
	}

	resourcesToDelete := cmd.MergeResourcesByID(resourceLists...)

	// Prompt user to confirm deletion
	if !r.Confirm {
		var promptMsg string
		switch {
		case len(applications) > 0:
			promptMsg = fmt.Sprintf("The environment %s contains %d application(s) and %d deployed resource(s). Are you sure you want to delete the environment, its applications and its resources?",
				r.EnvironmentName, len(applications), len(resourcesToDelete))
		case len(resourcesToDelete) > 0:
			promptMsg = fmt.Sprintf("The environment %s contains %d deployed resource(s). Are you sure you want to delete the environment and its resources?",
				r.EnvironmentName, len(resourcesToDelete))
		default:
			promptMsg = fmt.Sprintf("The environment %s is empty. Are you sure you want to delete the environment?",
				r.EnvironmentName)
		}

		confirmed, err := prompt.YesOrNoPrompt(promptMsg, prompt.ConfirmNo, r.InputPrompter)
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}

	if r.Force {
		r.Output.LogInfo(msgForceWarning)
	}

	if len(resourcesToDelete) > 0 {
		r.Output.LogInfo(msgDeletingResources, len(resourcesToDelete), r.EnvironmentName)

		if err := cmd.DeleteResourcesInParallel(ctx, managementClient, r.Output, resourcesToDelete, r.Force); err != nil {
			return clierrors.Message("Failed to delete resources in environment '%s': %v", r.EnvironmentName, err)
		}
	}

	// Applications are deleted after their resources, so that an interrupted delete leaves the
	// application behind as a recovery point rather than orphaning its resources.
	if len(applications) > 0 {
		r.Output.LogInfo(msgDeletingApplications, len(applications), r.EnvironmentName)

		appClient := r.RadiusCoreClientFactory.NewApplicationsClient()
		for _, application := range applications {
			if application.Name == nil {
				continue
			}

			r.Output.LogInfo(msgDeletingApplication, *application.Name)

			_, err := appClient.Delete(ctx, r.Workspace.Scope, *application.Name, &corerpv20250801.ApplicationsClientDeleteOptions{})
			if err != nil && !clients.Is404Error(err) {
				return clierrors.Message("Failed to delete application '%s' in environment '%s': %v", *application.Name, r.EnvironmentName, err)
			}
		}
	}

	_, err = envClient.Delete(ctx, r.Workspace.Scope, r.EnvironmentName, &corerpv20250801.EnvironmentsClientDeleteOptions{})
	if clients.Is404Error(err) {
		r.Output.LogInfo(msgEnvironmentNotFoundPreview, r.EnvironmentName)
		return nil
	} else if err != nil {
		return err
	}

	r.Output.LogInfo(msgEnvironmentDeletedPreview, r.EnvironmentName)

	return nil
}
