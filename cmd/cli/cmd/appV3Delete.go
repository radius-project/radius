// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/cli"
	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/Azure/radius/pkg/cli/environments"
	"github.com/Azure/radius/pkg/cli/prompt"
	"github.com/spf13/cobra"
)

// appV3DeleteCmd command to delete an applicationV3
var appV3DeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete RAD application",
	Long:  "Delete the specified RAD application deployed in the default environment",
	RunE:  deleteApplicationV3,
}

func init() {
	applicationV3Cmd.AddCommand(appV3DeleteCmd)

	appV3DeleteCmd.Flags().BoolP("yes", "y", false, "Use this flag to prevent prompt for confirmation")
}

func deleteApplicationV3(cmd *cobra.Command, args []string) error {
	yes, err := cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}

	config := ConfigFromContext(cmd.Context())
	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	applicationName, err := cli.RequireApplicationArgs(cmd, args, env)
	if err != nil {
		return err
	}

	// Prompt user to confirm deletion
	if !yes {
		confirmed, err := prompt.Confirm(fmt.Sprintf("Are you sure you want to delete '%v' from '%v' [y/n]?", applicationName, env.GetName()))
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}

	client, err := environments.CreateManagementClient(cmd.Context(), env)
	if err != nil {
		return err
	}

	err = appV3DeleteInner(cmd.Context(), client, applicationName, env)
	if err != nil {
		return err
	}

	err = updateApplicationConfig(config, env, applicationName)
	if err != nil {
		return err
	}

	return err
}

// appV3DeleteInner deletes a v3 application without argument/flag validation.
func appV3DeleteInner(ctx context.Context, client clients.ManagementClient, applicationName string, env environments.Environment) error {
	err := client.DeleteApplicationV3(ctx, applicationName)
	if err != nil {
		return fmt.Errorf("delete application error: %w", err)
	}

	fmt.Printf("Application '%s' has been deleted.\n", applicationName)
	return nil
}
