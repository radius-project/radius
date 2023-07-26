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

package register

import (
	"github.com/project-radius/radius/pkg/cli/cmd/credential/common"
	credential_register_aws "github.com/project-radius/radius/pkg/cli/cmd/credential/register/aws"
	credential_register_azure "github.com/project-radius/radius/pkg/cli/cmd/credential/register/azure"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command for the `rad credential create` command.
//
// # Function Explanation
//
// NewCommand() creates a new command for registering cloud provider credentials and adds subcommands for Azure and AWS.
func NewCommand(factory framework.Factory) *cobra.Command {
	// This command is not runnable, and thus has no runner.
	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register(Add or update) cloud provider credential for a Radius installation.",
		Long:  "Register (Add or update) cloud provider configuration for a Radius installation." + common.LongDescriptionBlurb,
		Example: `
# Register (Add or update) cloud provider credential for Azure with service principal authentication
rad credential register azure --client-id <client id> --client-secret <client secret> --tenant-id <tenant id> --subscription <subscription id> --resource-group <resource group name>		
`,
	}

	azure, _ := credential_register_azure.NewCommand(factory)
	cmd.AddCommand(azure)

	aws, _ := credential_register_aws.NewCommand(factory)
	cmd.AddCommand(aws)

	return cmd
}
