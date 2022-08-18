// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package show

import (
	"context"

	"github.com/project-radius/radius/cmd/rad/cmd"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/utils"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewCommand() *cobra.Command {
	runner := NewRunner()

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

	cmd.PersistentFlags().StringP("type", "t", "", "The resource type")
	cmd.PersistentFlags().StringP("resource", "r", "", "The resource name")
	cmd.Flags().StringP("resource-group", "g", "", "Resource Group of the resource. This parameter is required if the resource type is a Microsoft Azure resource.")
	cmd.Flags().StringP("resource-subscription-id", "s", "", "Subscription id of the resource. This parameter is required if the resource type is a Microsoft Azure resource.")

	return cmd
}

type Runner struct {
	Config            *viper.Viper
	Workspace         *workspaces.Workspace
	ConnectionFactory connections.Factory
	ResourceType      string
	ResourceName      string
	Format            string
}

func NewRunner() *Runner {
	return &Runner{
		ConnectionFactory: connections.DefaultFactory,
	}
}

func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	// TODO: get config
	config := utils.ConfigFromContext(cmd.Context())
	r.Config = config
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
	err = output.Write(r.Format, resourceDetails, cmd.RootCmd.OutOrStdout(), objectformats.GetResourceTableFormat())
	if err != nil {
		return err
	}

	return nil
}
