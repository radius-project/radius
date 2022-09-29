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
		Long:  `Manage workspaces`,
		Example: `
# List resource groups in default workspace
rad workspace list

# Create workspace 
rad workspace create localWorkspace -g prodrg -e prodenv -context kind-kind

# Delete workspace
rad workspace delete localWorkspace

# Show details of workspace
rad workspace show localWorkspace

#switch workspace
rad workspace switch newworkspace
`,
	}

	create, _ := workspace_create.NewCommand(factory)
	cmd.AddCommand(create)

	return cmd

}
