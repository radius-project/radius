// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package show

import (
	"context"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := &Runner{
		Config:            viper.New(), // TODO: config for real.
		ConnectionFactory: factory.GetConnectionFactory(),
	}

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

	return cmd, runner
}

type Runner struct {
	Config            *viper.Viper
	Workspace         *workspaces.Workspace
	ConnectionFactory connections.Factory
	ResourceType      string
	ResourceName      string
	Format            string
}

func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	// TODO: get config

	workspace, err := cli.RequireWorkspace(cmd, r.Config)
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
	client, err := connections.DefaultFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	resourceDetails, err := client.ShowResource(ctx, r.ResourceType, r.ResourceName)
	if err != nil {
		return err
	}

	// TODO: create a mock for output.
	err = output.Write(r.Format, resourceDetails, cmd.OutOrStdout(), objectformats.GetResourceTableFormat())
	if err != nil {
		return err
	}

	return nil
}
