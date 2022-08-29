// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"strings"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

// appSwitchCmd command to switch applications
var appSwitchCmd = &cobra.Command{
	Use:   "switch",
	Short: "Switch the default RAD application",
	Long:  "Switches the default RAD application",
	RunE:  switchApplications,
}

func init() {
	applicationCmd.AddCommand(appSwitchCmd)
}

func switchApplications(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	workspace, err := cli.RequireWorkspace(cmd, config)
	if err != nil {
		return err
	}

	applicationName, err := cli.ReadApplicationNameArgs(cmd, args)
	if err != nil {
		return err
	}

	// HEY YOU: Keep the logic below here in sync with `rad env switch``
	if strings.EqualFold(workspace.DefaultApplication, applicationName) {
		output.LogInfo("Default application is already set to %v", applicationName)
		return nil
	}

	if workspace.DefaultApplication == "" {
		output.LogInfo("Switching default application to %v", applicationName)
	} else {
		output.LogInfo("Switching default application from %v to %v", workspace.DefaultApplication, applicationName)
	}

	err = cli.EditWorkspaces(cmd.Context(), config, func(section *cli.WorkspaceSection) error {
		workspace, err := section.GetWorkspace(workspace.Name)
		if err != nil {
			return err
		}

		workspace.DefaultApplication = applicationName
		section.Items[strings.ToLower(workspace.Name)] = *workspace
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
