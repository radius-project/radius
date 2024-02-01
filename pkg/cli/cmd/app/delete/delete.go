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

package list

import (
	"context"
	"fmt"
	"strings"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/spf13/cobra"
)

const (
	deleteConfirmation = "Are you sure you want to delete application '%v' from environment '%v'?"
	bicepWarning       = "'%v' is a Bicep filename or path and not the name of a Radius Application. Specify the name of a valid application and try again"
)

// NewCommand creates an instance of the `rad app delete` command and runner.
//

// NewCommand creates a new Cobra command for deleting a Radius Application, with flags for workspace, resource group,
// application name and confirmation, and returns the command and a Runner object.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete Radius Application",
		Long:  "Delete the specified Radius Application deployed in the default environment",
		Example: `
# Delete current application
rad app delete

# Delete current application and bypass confirmation prompt
rad app delete --yes

# Delete specified application
rad app delete my-app

# Delete specified application in a specified resource group
rad app delete my-app --group my-group
`,
		Args: cobra.MaximumNArgs(1),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddApplicationNameFlag(cmd)
	commonflags.AddConfirmationFlag(cmd)

	return cmd, runner
}

// Runner is the Runner implementation for the `rad app delete` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	InputPrompter     prompt.Interface
	Output            output.Interface

	ApplicationName string
	EnvironmentName string
	Scope           string
	Confirm         bool
	Workspace       *workspaces.Workspace
}

// NewRunner creates an instance of the runner for the `rad app delete` command.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		ConnectionFactory: factory.GetConnectionFactory(),
		InputPrompter:     factory.GetPrompter(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad app delete` command.
//

// Validate checks the workspace, scope, application name, and confirm flag from the command line arguments and
// request object, and returns an error if any of these are invalid.
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

	r.ApplicationName, err = cli.RequireApplicationArgs(cmd, args, *workspace)
	if err != nil {
		return err
	}

	// Lookup the environment name for use in the confirmation prompt
	if workspace.Environment != "" {
		id, err := resources.ParseResource(workspace.Environment)
		if err != nil {
			return err
		}

		r.EnvironmentName = id.Name()
	}

	// Throw error if user specifies a Bicep filename or path instead of an application name
	if strings.HasSuffix(r.ApplicationName, ".bicep") {
		return clierrors.Message(bicepWarning, r.ApplicationName)
	}

	r.Confirm, err = cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}

	return nil
}

// Run runs the `rad app delete` command.
//

// "Run" prompts the user to confirm the deletion of an application, creates a connection to the application management
// client, and deletes the application if it exists. If the application does not exist, it logs a message. It returns an
// error if there is an issue with the connection or the prompt.
func (r *Runner) Run(ctx context.Context) error {
	// Prompt user to confirm deletion
	if !r.Confirm {
		confirmed, err := prompt.YesOrNoPrompt(fmt.Sprintf(deleteConfirmation, r.ApplicationName, r.EnvironmentName), prompt.ConfirmNo, r.InputPrompter)
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}

	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	deleted, err := client.DeleteApplication(ctx, r.ApplicationName)
	if err != nil {
		return err
	}

	if deleted {
		r.Output.LogInfo("Application %s deleted", r.ApplicationName)
	} else {
		r.Output.LogInfo("Application '%s' does not exist or has already been deleted.", r.ApplicationName)
	}

	return nil
}
