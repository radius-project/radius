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
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/objectformats"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad resource show` command.
//

// NewCommand creates a new Cobra command and a new Runner, and configures the command with the Runner, common flags, and
//
//	usage information.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "show [resourceType] [resourceName]",
		Short: "Show Radius resource details",
		Long:  "Show details of the specified Radius resource",
		Example: `
	sample list of resourceType: containers, gateways, httpRoutes, daprPubSubBrokers, extenders, mongoDatabases, rabbitMQMessageQueues, redisCaches, sqlDatabases, daprStateStores, daprSecretStores

	# show details of a specified resource in the default environment

	rad resource show containers orders
	rad resource show gateways orders_gateways
	rad resource show httpRoutes orders_routes

	# show details of a specified resource in an application
	rad resource show containers orders --application icecream-store
	
	# show details of a specified resource in an application (shorthand flag)
	rad resource show containers orders -a icecream-store 
	`,
		Args: cobra.ExactArgs(2),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad resource show` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface
	Workspace         *workspaces.Workspace
	ResourceType      string
	ResourceName      string
	Format            string
}

// NewRunner creates a new instance of the `rad resource show` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad resource show` command.
//

// Validate checks the workspace, scope, resource type and name, and output format from the command line arguments and config,
// and sets them in the Runner struct. It returns an error if any of these values are not valid.
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

	resourceType, resourceName, err := cli.RequireResourceTypeAndName(args)
	if err != nil {
		return err
	}
	r.ResourceType = resourceType
	r.ResourceName = resourceName

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	return nil
}

// Run runs the `rad resource show` command.
//

// Run creates a connection to an applications management client, retrieves resource details, and writes the details in a
// specified format to an output. It returns an error if any of these steps fail.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	resourceDetails, err := client.ShowResource(ctx, r.ResourceType, r.ResourceName)
	if err != nil {
		return err
	}

	err = r.Output.WriteFormatted(r.Format, resourceDetails, objectformats.GetResourceTableFormat())
	if err != nil {
		return err
	}

	return nil
}
