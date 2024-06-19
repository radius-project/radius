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
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

const (
	deleteConfirmation = "Are you sure you want to delete resource provider %q?"
)

// NewCommand creates an instance of the `rad resourceprovider delete` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "delete [resource provider namespace]",
		Short: "Delete resource provider",
		Long: `Delete resource provider
		
Resource providers are the entities that implement resource types such as 'Applications.Core/containers'. Resource providers can be defined, registered, and unregistered by users.

Built-in resource providers like 'Applications.Core' can be deleted.

Deleting a resource provider will delete all resources of the resource provider.`,
		Example: `
# Delete a resource provider
rad resourceprovider delete Applications.Core`,
		Args: cobra.ExactArgs(1),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddConfirmationFlag(cmd)
	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)

	return cmd, runner
}

// Runner is the Runner implementation for the `rad resourceprovider delete` command.
type Runner struct {
	ConnectionFactory connections.Factory
	ConfigHolder      *framework.ConfigHolder
	InputPrompter     prompt.Interface
	Output            output.Interface
	Format            string
	Workspace         *workspaces.Workspace

	Confirm                   bool
	ResourceProviderNamespace string
}

// NewRunner creates an instance of the runner for the `rad resourceprovider delete` command.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		InputPrompter:     factory.GetPrompter(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad resourceprovider delete` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	r.Confirm, err = cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}

	r.ResourceProviderNamespace = args[0]

	return nil
}

// Run runs the `rad resourceprovider delete` command.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	// Prompt user to confirm deletion
	if !r.Confirm {
		confirmed, err := prompt.YesOrNoPrompt(fmt.Sprintf(deleteConfirmation, r.ResourceProviderNamespace), prompt.ConfirmNo, r.InputPrompter)
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}

	deleted, err := client.DeleteResourceProvider(ctx, "local", r.ResourceProviderNamespace)
	if clients.Is404Error(err) {
		return clierrors.Message("The resource provider %q was not found or has been deleted.", r.ResourceProviderNamespace)
	} else if err != nil {
		return err
	}

	if deleted {
		r.Output.LogInfo("Resource provider %q deleted.", r.ResourceProviderNamespace)
	} else {
		r.Output.LogInfo("Resource provider %q does not exist or has already been deleted.", r.ResourceProviderNamespace)
	}

	return nil
}
