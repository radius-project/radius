// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package create

import (
	"github.com/project-radius/radius/pkg/cli/cmd/provider/common"
	provider_create_azure "github.com/project-radius/radius/pkg/cli/cmd/provider/create/azure"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command for the `rad provider create` command.
func NewCommand(factory framework.Factory) *cobra.Command {
	// This command is not runnable, and thus has no runner.
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Add or update cloud provider configuration for a Radius installation.",
		Long:  "Add or update cloud provider configuration for a Radius installation." + common.LongDescriptionBlurb,
		Example: `
# Add or update cloud provider configuration for Azure with service principal authentication
rad provider create azure --client-id <client id> --client-secret <client secret> --tenant-id <tenant id> --subscription <subscription id> --resource-group <resource group name>		
`,
	}

	azure, _ := provider_create_azure.NewCommand(factory)
	cmd.AddCommand(azure)

	return cmd
}
