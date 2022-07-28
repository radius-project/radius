// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"strings"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/spf13/cobra"
)

var envSwitchCmd = &cobra.Command{
	Use:   "switch [environment]",
	Short: "Switch the current environment",
	Long:  "Switch the current environment",
	RunE:  switchEnv,
}

func init() {
	envCmd.AddCommand(envSwitchCmd)
}

func switchEnv(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	workspace, err := cli.RequireWorkspace(cmd, config)
	if err != nil {
		return err
	}

	environmentName, err := cli.RequireEnvironmentNameArgs(cmd, args, *workspace)
	if err != nil {
		return err
	}

	// TODO: for right now we assume the environment is in the default resource group.
	scope, err := resources.Parse(workspace.Scope)
	if err != nil {
		return err
	}

	id := scope.Append(resources.TypeSegment{Type: "Applications.Core/environments", Name: environmentName})

	// HEY YOU: Keep the logic below here in sync with `rad app switch`
	if strings.EqualFold(workspace.Environment, id.String()) {
		output.LogInfo("Default environment is already set to %v", environmentName)
		return nil
	}

	if workspace.Environment == "" {
		output.LogInfo("Switching default environment to %v", environmentName)
	} else {
		// Parse the environment ID to get the name
		existing, err := resources.Parse(workspace.Environment)
		if err != nil {
			return err
		}

		output.LogInfo("Switching default environment from %v to %v", existing.Name(), environmentName)
	}

	err = cli.EditWorkspaces(cmd.Context(), config, func(section *cli.WorkspaceSection) error {
		workspace, err := section.GetWorkspace(workspace.Name)
		if err != nil {
			return err
		}

		workspace.Environment = id.String()
		section.Items[strings.ToLower(workspace.Name)] = *workspace
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
