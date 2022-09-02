// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package workspace

import (
	workspace_create "github.com/project-radius/radius/pkg/cli/cmd/workspace/create"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/spf13/cobra"
)

func NewCommand(factory framework.Factory) *cobra.Command {
	// This command is not runnable, and thus has no runner.
	cmd := &cobra.Command{
		Use:   "workspace",
		Short: "Manage workspaces",
		Long: `Manage workspaces
		Workspaces allow you to manage multiple Radius platforms and environments using a local configuration file. 
		You can easily define and switch between workspaces to deploy and manage applications across local, test, and production environments.
		`,
		Example: `
		# List all local workspaces
		rad workspace list

		# Create workspace with no default resource group or environment set
		rad workspace create kubernetes myworkspace --context kind-kind

		# Create workspace with default resource group and environment set
		rad workspace create kubernetes myworkspace --context kind-kind --group myrg --environment myenv

		# Delete workspace
		rad workspace delete myworkspace

		# Show details of workspace
		rad workspace show myWorkspace

		#switch workspace
		rad workspace switch otherWorkspace
		`,
	}

	create, _ := workspace_create.NewCommand(factory)
	cmd.AddCommand(create)

	return cmd

}
