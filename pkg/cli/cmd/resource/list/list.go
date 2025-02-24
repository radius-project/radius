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

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/cmd/resourcetype/common"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/objectformats"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad resource list` command.
//

// NewCommand creates a new Cobra command and a Runner to list resources of a specified type in an application or the
// default environment, and adds flags for application name, resource group, output and workspace.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "list [resourceType]",
		Short: "Lists resources",
		Long:  "List all resources of specified type",
		Example: `
sample list of resourceType: Applications.Core/containers, Applications.Core/gateways, Applications.Dapr/daprPubSubBrokers, Applications.Core/extenders, Applications.Datastores/mongoDatabases, Applications.Messaging/rabbitMQMessageQueues, Applications.Datastores/redisCaches, Applications.Datastores/sqlDatabases, Applications.Dapr/daprStateStores, Applications.Dapr/daprSecretStores

# list all resources of a specified type in the default environment

rad resource list Applications.Core/containers
rad resource list Applications.Core/gateways

# list all resources of a specified type in an application
rad resource list Applications.Core/containers --application icecream-store

# list all resources of a specified type in an application (shorthand flag)
rad resource list Applications.Core/containers -a icecream-store
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
	ConfigHolder              *framework.ConfigHolder
	ConnectionFactory         connections.Factory
	Output                    output.Interface
	Workspace                 *workspaces.Workspace
	ApplicationName           string
	Format                    string
	ResourceType              string
	ResourceTypeSuffix        string
	ResourceProviderNameSpace string
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
//

// Validate checks the command line args, workspace, scope, application name, resource type and output format, and
// returns an error if any of these are invalid.
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

	r.ResourceProviderNameSpace, r.ResourceTypeSuffix, err = cli.RequireFullyQualifiedResourceType(args)
	if err != nil {
		return err
	}
	r.ResourceType = r.ResourceProviderNameSpace + "/" + r.ResourceTypeSuffix

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	return nil
}

// Run runs the `rad resource list` command.
//

// Run checks if an application name is provided and if so, checks if the application exists in the workspace, then
// lists all resources of the specified type in the application, and finally writes the resources to the output in the
// specified format. If no application name is provided, it lists all resources of the specified type. An error is
// returned if the application does not exist in the workspace.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	var resourceList []generated.GenericResource

	_, err = common.GetResourceTypeDetails(ctx, r.ResourceProviderNameSpace, r.ResourceTypeSuffix, client)
	if err != nil {
		return err
	}

	if r.ApplicationName == "" {
		resourceList, err = client.ListResourcesOfType(ctx, r.ResourceType)
		if err != nil {
			return err
		}
	} else {
		_, err = client.GetApplication(ctx, r.ApplicationName)
		if clients.Is404Error(err) {
			return clierrors.Message("The application %q could not be found in workspace %q. Make sure you specify the correct application with '-a/--application'.", r.ApplicationName, r.Workspace.Name)
		} else if err != nil {
			return err
		}

		resourceList, err = client.ListResourcesOfTypeInApplication(ctx, r.ApplicationName, r.ResourceType)
		if err != nil {
			return err
		}
	}

	return r.Output.WriteFormatted(r.Format, resourceList, objectformats.GetGenericResourceTableFormat())
}
