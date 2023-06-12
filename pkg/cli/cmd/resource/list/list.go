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
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/clierrors"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad resource list` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "list [resourceType]",
		Short: "Lists resources",
		Long:  "List all resources of specified type",
		Example: `
	sample list of resourceType: containers, gateways, httpRoutes, daprPubSubBrokers, daprInvokeHttpRoutes, extenders, mongoDatabases, rabbitMQMessageQueues, redisCaches, sqlDatabases, daprStateStores, daprSecretStores

	# list all resources of a specified type in the default environment

	rad resource list containers
	rad resource list gateways
	rad resource list httpRoutes

	# list all resources of a specified type in an application
	rad resource list containers --application icecream-store
	
	# list all resources of a specified type in an application (shorthand flag)
	rad resource list containers -a icecream-store
	`,
		Args: cobra.ExactArgs(1),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddApplicationNameFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad resource list` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface
	Workspace         *workspaces.Workspace
	ApplicationName   string
	Format            string
	ResourceType      string
}

// NewRunner creates a new instance of the `rad resource list` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		ConnectionFactory: factory.GetConnectionFactory(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad resource list` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	// Validate command line args and
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

	applicationName, err := cli.ReadApplicationName(cmd, *workspace)
	if err != nil {
		return err
	}
	r.ApplicationName = applicationName

	resourceType, err := cli.RequireResourceType(args)
	if err != nil {
		return err
	}
	r.ResourceType = resourceType

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	return nil
}

// Run runs the `rad resource list` command.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	if r.ApplicationName == "" {
		resourceList, err := client.ListAllResourcesByType(ctx, r.ResourceType)
		if err != nil {
			return err
		}

		err = r.Output.WriteFormatted(r.Format, resourceList, objectformats.GetResourceTableFormat())
		if err != nil {
			return err
		}
		return nil
	} else {
		_, err = client.ShowApplication(ctx, r.ApplicationName)
		if clients.Is404Error(err) {
			return clierrors.Message("The application %q could not be found in workspace %q.", r.ApplicationName, r.Workspace.Name)
		} else if err != nil {
			return err
		}

		resourceList, err := client.ListAllResourcesOfTypeInApplication(ctx, r.ApplicationName, r.ResourceType)
		if err != nil {
			return err
		}

		err = r.Output.WriteFormatted(r.Format, resourceList, objectformats.GetResourceTableFormat())
		if err != nil {
			return err
		}
		return nil
	}
}
