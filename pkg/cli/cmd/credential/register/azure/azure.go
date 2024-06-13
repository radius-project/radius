/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package azure

import (
	"github.com/radius-project/radius/pkg/cli/cmd/credential/common"
	credential_register_azure_sp "github.com/radius-project/radius/pkg/cli/cmd/credential/register/azure/sp"
	credential_register_azure_wi "github.com/radius-project/radius/pkg/cli/cmd/credential/register/azure/wi"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command for the `rad credential register azure` command.
// This command is not runnable, but contains subcommands for registering Azure cloud provider credentials.
func NewCommand(factory framework.Factory) *cobra.Command {
	// This command is not runnable, and thus has no runner.
	cmd := &cobra.Command{
		Use:   "azure",
		Short: "Register (Add or update) Azure cloud provider credential for a Radius installation.",
		Long:  "Register (Add or update) Azure cloud provider credential for a Radius installation." + common.LongDescriptionBlurb,
		Example: `
# Register (Add or update) cloud provider credential for Azure with service principal authentication
rad credential register azure sp --client-id <client id> --client-secret <client secret> --tenant-id <tenant id>
# Register (Add or update) cloud provider credential for Azure with workload identity authentication
rad credential register azure wi --client-id <client id> --tenant-id <tenant id>
`,
	}

	azureSP, _ := credential_register_azure_sp.NewCommand(factory)
	cmd.AddCommand(azureSP)

	azureWI, _ := credential_register_azure_wi.NewCommand(factory)
	cmd.AddCommand(azureWI)

	return cmd
}
