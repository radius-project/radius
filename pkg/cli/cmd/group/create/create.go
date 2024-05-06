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

package create

import (
	"context"

	"github.com/spf13/cobra"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/cmd/group/common"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
)

// NewCommand creates an instance of the command and runner for the `rad group create` command.
//

// NewCommand creates a new Cobra command and a Runner to handle the command's logic, which is used to create a new
// resource group.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "create resourcegroupname",
		Short: "Create a new resource group",
		Long: `Create a new resource group

Resource groups are used to organize and manage Radius resources. They often contain resources that share a common lifecycle or unit of deployment.

A Radius Application and its resources can span one or more resource groups, and do not have to be in the same resource group as the Radius Environment into which it's being deployed into.

Note that these resource groups are separate from the Azure cloud provider and Azure resource groups configured with the cloud provider.
`,
		Example: `rad group create rgprod`,
		Args:    cobra.MaximumNArgs(1),
		RunE:    framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad group create` command.
type Runner struct {
	ConfigHolder         *framework.ConfigHolder
	ConnectionFactory    connections.Factory
	Output               output.Interface
	Workspace            *workspaces.Workspace
	UCPResourceGroupName string
}

// NewRunner creates a new instance of the `rad group create` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad group create` command.
//

// Validate checks if the given command and arguments are valid, and if so, sets the resource group name and workspace in the
// Runner struct. It returns an error if the resource group name is invalid or if the workspace or resource group cannot be found.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}

	resourceGroup, err := cli.RequireUCPResourceGroup(cmd, args)
	if err != nil {
		return err
	}

	err = common.ValidateResourceGroupName(resourceGroup)
	if err != nil {
		return err
	}

	r.UCPResourceGroupName = resourceGroup
	r.Workspace = workspace

	return nil
}

// Run runs the `rad group create` command.
//

// Run creates a resource group in the workspace and returns an error if unsuccessful.
func (r *Runner) Run(ctx context.Context) error {

	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	r.Output.LogInfo("creating resource group %q in workspace %q...\n", r.UCPResourceGroupName, r.Workspace.Name)

	err = client.CreateUCPGroup(ctx, "local", r.UCPResourceGroupName, v20231001preview.ResourceGroupResource{
		Location: to.Ptr(v1.LocationGlobal),
	})
	if err != nil {
		return err
	}

	r.Output.LogInfo("resource group %q created", r.UCPResourceGroupName)
	return nil

}
