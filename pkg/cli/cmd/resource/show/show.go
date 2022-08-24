// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package show

import (
	"context"
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/shared"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "show [resourceType] [resourceName]",
		Short: "Show RAD resource details",
		Long:  "Show details of the specified Radius resource",
		Example: `
	sample list of resourceType: containers, gateways, httpRoutes, daprPubSubBrokers, daprInvokeHttpRoutes, extenders, mongoDatabases, rabbitMQMessageQueues, redisCaches, sqlDatabases, daprStateStores, daprSecretStores

	# show details of a specified resource in the default environment

	rad resource show containers orders
	rad resource show gateways orders_gateways
	rad resource show httpRoutes orders_routes

	# show details of a specified resource in an application
	rad resource show containers orders --application icecream-store
	
	# show details of a specified resource in an application (shorthand flag)
	rad resource show containers orders -a icecream-store 
	`,
		Args: cobra.ExactArgs(0),

		RunE: framework.RunCommand(runner),
	}

	outputDescription := fmt.Sprintf("output format (supported formats are %s)", strings.Join(output.SupportedFormats(), ", "))
	cmd.Flags().StringP("workspace", "w", "", "The workspace name")
	cmd.Flags().StringP("output", "o", "", outputDescription)
	cmd.Flags().StringP("type", "t", "", "The resource type")
	cmd.Flags().StringP("resource", "r", "", "The resource name")

	return cmd, runner
}

type Runner struct {
	ConfigHolder      *shared.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface
	Workspace         *workspaces.Workspace
	ResourceType      string
	ResourceName      string
	Format            string
}

func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
	}
}

func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config)
	if err != nil {
		return err
	}
	r.Workspace = workspace

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

func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	resourceDetails, err := client.ShowResource(ctx, r.ResourceType, r.ResourceName)
	if err != nil {
		return err
	}

	err = r.Output.Write(r.Format, resourceDetails, objectformats.GetResourceTableFormat())
	if err != nil {
		return err
	}

	return nil
}
