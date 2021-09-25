// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"

	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/azure/radclientv3"
	"github.com/Azure/radius/pkg/cli"
	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/Azure/radius/pkg/cli/environments"
	"github.com/Azure/radius/pkg/cli/objectformats"
	"github.com/Azure/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

// componentListCmd command to list components in an application
var componentListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists application components",
	Long:  "List all the components in the specified application",
	RunE:  listComponents,
}

func init() {
	componentCmd.AddCommand(componentListCmd)
}

func listComponents(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	applicationName, err := cli.RequireApplicationArgs(cmd, args, env)
	if err != nil {
		return err
	}

	client, err := environments.CreateManagementClient(cmd.Context(), env)
	if err != nil {
		return err
	}

	var results interface{}
	var objectfmt output.FormatterOptions
	if cli.V3(cmd) {
		results, err = componentListV3(cmd.Context(), client, applicationName)
		if err != nil {
			return err
		}
		objectfmt = objectformats.GetResourceTableFormat()
	} else {
		results, err = componentList(cmd.Context(), client, applicationName)
		if err != nil {
			return err
		}
		objectfmt = objectformats.GetComponentTableFormat()
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	err = output.Write(format, results, cmd.OutOrStdout(), objectfmt)
	if err != nil {
		return err
	}

	return nil
}

func componentList(ctx context.Context, client clients.ManagementClient, applicationName string) ([]*radclient.ComponentResource, error) {
	l, err := client.ListComponents(ctx, applicationName)
	if err != nil {
		return nil, err
	}
	return l.Value, err
}

func componentListV3(ctx context.Context, client clients.ManagementClient, applicationName string) ([]*radclientv3.RadiusResource, error) {
	l, err := client.ListResourcesV3(ctx, applicationName)
	if err != nil {
		return nil, err
	}
	return l.Value, err
}
