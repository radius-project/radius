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

package groupswitch

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad group switch` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "switch resourcegroupname",
		Short: "Switch default resource group scope",
		Long: `Switch default resource group scope
	
	Radius workspaces contain a resource group scope, where Radius applications and resources are deployed by default. The switch command changes the default scope of the workspace to the specified resource group name.
	
	Resource groups are used to organize and manage Radius resources. They often contain resources that share a common lifecycle or unit of deployment.
			
	Note that these resource groups are separate from the Azure cloud provider and Azure resource groups configured with the cloud provider.`,
		Example: `rad group switch rgprod -w wsprod`,
		Args:    cobra.MaximumNArgs(1),
		RunE:    framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad group switch` command.
type Runner struct {
	ConfigHolder         *framework.ConfigHolder
	ConnectionFactory    connections.Factory
	Workspace            *workspaces.Workspace
	UCPResourceGroupName string
}

// NewRunner creates a new instance of the `rad group switch` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
	}
}

// Validate runs validation for the `rad group switch` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}

	if !workspace.IsEditableWorkspace() {
		// Only workspaces stored in configuration can be modified.
		return workspaces.ErrEditableWorkspaceRequired
	}

	resourceGroup, err := cli.RequireUCPResourceGroup(cmd, args)
	if err != nil {
		return err
	}

	r.UCPResourceGroupName = resourceGroup
	r.Workspace = workspace

	return nil
}

// Run runs the `rad group switch` command.
func (r *Runner) Run(ctx context.Context) error {

	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	_, err = client.ShowUCPGroup(ctx, "radius", "local", r.UCPResourceGroupName)
	if err != nil {
		return &cli.FriendlyError{Message: fmt.Sprintf("resource group %q does not exist. Run `rad group create` or `rad init` and try again \n", r.UCPResourceGroupName)}
	}

	scope := fmt.Sprintf("/planes/radius/local/resourceGroups/%s", r.UCPResourceGroupName)

	err = cli.EditWorkspaces(ctx, r.ConfigHolder.Config, func(section *cli.WorkspaceSection) error {

		workspace := *r.Workspace
		workspace.Scope = scope
		section.Items[workspace.Name] = workspace

		return nil
	})
	return err

}
