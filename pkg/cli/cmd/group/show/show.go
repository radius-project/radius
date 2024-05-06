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

package show

import (
	"context"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/cmd/group/common"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad group show` command.
//

// NewCommand creates a new cobra command for showing the details of a resource group, with flags for resource group,
// workspace, and output. It returns the command and a Runner to execute the command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "show resourcegroupname",
		Short: "Show the details of a resource group",
		Long: `Show the details of a resource group

Resource groups are used to organize and manage Radius resources. They often contain resources that share a common lifecycle or unit of deployment.

A Radius Application and its resources can span one or more resource groups, and do not have to be in the same resource group as the Radius Environment into which it's being deployed into.

Note that these resource groups are separate from the Azure cloud provider and Azure resource groups configured with the cloud provider.
`,
		Example: `rad group show rgprod`,
		Args:    cobra.MaximumNArgs(1),
		RunE:    framework.RunCommand(runner),
	}

	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddOutputFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad group show` command.
type Runner struct {
	ConfigHolder         *framework.ConfigHolder
	ConnectionFactory    connections.Factory
	Output               output.Interface
	Workspace            *workspaces.Workspace
	UCPResourceGroupName string
	ResourceType         string
	ResourceName         string
	Format               string
}

// NewRunner creates a new instance of the `rad group show` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad group show` command.
//

// Validate checks the command line arguments, workspace, output format, and resource group, and sets the
// corresponding fields in the Runner struct. It returns an error if any of these checks fail.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	if format == "" {
		format = "table"
	}
	resourcegroup, err := cli.RequireUCPResourceGroup(cmd, args)
	if err != nil {
		return err
	}

	r.Format = format
	r.UCPResourceGroupName = resourcegroup
	r.Workspace = workspace

	return nil
}

// Run runs the `rad group show` command.
//

// Run creates an ApplicationsManagementClient, retrieves a resource group, and writes the resource group to an
// output, returning an error if any of these steps fail.
func (r *Runner) Run(ctx context.Context) error {

	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	resourceGroup, err := client.ShowUCPGroup(ctx, "local", r.UCPResourceGroupName)
	if err != nil {
		return err
	}

	err = r.Output.WriteFormatted(r.Format, resourceGroup, common.ResourceGroupFormat())

	if err != nil {
		return err
	}
	return err

}
