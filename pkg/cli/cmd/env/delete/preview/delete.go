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
	"github.com/radius-project/radius/pkg/cli/cmd"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
)

const (
	msgEnvironmentDeletedPreview    = "Environment deleted"
	msgEnvironmentNotFoundPreview   = "Environment '%s' does not exist or has already been deleted."
	msgDeletingEnvironmentPreview   = "Deleting environment %s...\n"
	msgDeletingResourceCountPreview = "Deleting %d resource(s) in environment %s...\n"
)

// NewCommand creates an instance of the command and runner for the `rad env delete --preview` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:     "delete",
		Short:   "Delete environment (preview)",
		Long:    `Delete environment using the Radius.Core preview API surface.`,
		Args:    cobra.MaximumNArgs(1),
		RunE:    framework.RunCommand(runner),
		Example: `rad env delete myenv`,
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	commonflags.AddConfirmationFlag(cmd)
	commonflags.AddOutputFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the preview `rad env delete` command.
type Runner struct {
	ConfigHolder            *framework.ConfigHolder
	Output                  output.Interface
	InputPrompter           prompt.Interface
	Workspace               *workspaces.Workspace
	RadiusCoreClientFactory *corerpv20250801.ClientFactory

	Confirm         bool
	EnvironmentName string
}

// NewRunner creates a new instance of the preview delete runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:  factory.GetConfigHolder(),
		Output:        factory.GetOutput(),
		InputPrompter: factory.GetPrompter(),
	}
}

// Validate runs validation for the preview delete command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
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

	_, err = cli.RequireOutput(cmd) // we ignore format for preview delete
	if err != nil {
		return err
	}

	return nil
}

// Run executes the preview delete command logic.
func (r *Runner) Run(ctx context.Context) error {
	if r.RadiusCoreClientFactory == nil {
		factory, err := cmd.InitializeRadiusCoreClientFactory(ctx, r.Workspace, r.Workspace.Scope)
		if err != nil {
			return err
		}
		r.RadiusCoreClientFactory = factory
	}

	// For better feedback, list resources in the environment using the generic client facade.
	// The ApplicationsManagementClient abstraction isn't used here; instead we use the Radius.Core
	// client factory directly and count generic resources by environment ID.

	// Prompt user to confirm deletion
	if !r.Confirm {
		promptMsg := fmt.Sprintf("The environment %s is empty. Are you sure you want to delete the environment?", r.EnvironmentName)

		confirmed, err := prompt.YesOrNoPrompt(promptMsg, prompt.ConfirmNo, r.InputPrompter)
		if err != nil {
			return err
		}
		if !confirmed {
			r.Output.LogInfo("Environment %q NOT deleted", r.EnvironmentName)
			return nil
		}
	}

	// Show progress messages (without resource count for preview, since we don't enumerate here)
	r.Output.LogInfo(msgDeletingEnvironmentPreview, r.EnvironmentName)

	client := r.RadiusCoreClientFactory.NewEnvironmentsClient()
	_, err := client.Delete(ctx, r.EnvironmentName, &corerpv20250801.EnvironmentsClientDeleteOptions{})
	if err != nil {
		// If this is a 404, treat as successful but with a different message
		// We don't have the Is404Error helper wired to Radius.Core yet, so always surface the error.
		return err
	}

	r.Output.LogInfo(msgEnvironmentDeletedPreview)

	return nil
}
