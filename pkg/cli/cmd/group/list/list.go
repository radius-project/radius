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

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad group list` command.
//
// # Function Explanation
//
// NewCommand creates a new cobra command that can be used to list resource groups within the current or specified workspace, and returns
// a Runner object that can be used to execute the command. It also adds workspace and output flags to the command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List resource groups within current/specified workspace",
		Long: `List resource groups within current/specified workspace
	
	Resource groups are used to organize and manage Radius resources. They often contain resources that share a common lifecycle or unit of deployment.
			
	A Radius application and its resources can span one or more resource groups, and do not have to be in the same resource group as the Radius environment into which it's being deployed into.
			
	Note that these resource groups are separate from the Azure cloud provider and Azure resource groups configured with the cloud provider.`,
		Example: `rad group list`,
		Args:    cobra.ExactArgs(0),
		RunE:    framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddOutputFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad group list` command.
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

// NewRunner creates a new instance of the `rad group list` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad group list` command.
//
// # Function Explanation
//
// Validate checks if a workspace is required and sets the output format, returning an error if either of these checks fail.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}

	format, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	}
	if format == "" {
		format = "table"
	}
	r.Format = format
	r.Workspace = workspace

	return nil
}

// Run runs the `rad group list` command.
//
// # Function Explanation
//
// Run creates an ApplicationsManagementClient, retrieves a list of UCPGroup resources, and writes the results to
// an output in a formatted way, returning an error if one occurs.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	resourceGroupDetails, err := client.ListUCPGroup(ctx, "radius", "local")
	if err != nil {
		return err
	}

	err = r.Output.WriteFormatted(r.Format, resourceGroupDetails, objectformats.GetResourceGroupTableFormat())

	if err != nil {
		return err
	}
	return err
}
