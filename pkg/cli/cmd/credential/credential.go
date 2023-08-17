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

package credential

import (
	"github.com/project-radius/radius/pkg/cli/cmd/credential/common"
	credential_list "github.com/project-radius/radius/pkg/cli/cmd/credential/list"
	credential_register "github.com/project-radius/radius/pkg/cli/cmd/credential/register"
	credential_show "github.com/project-radius/radius/pkg/cli/cmd/credential/show"
	credential_unregister "github.com/project-radius/radius/pkg/cli/cmd/credential/unregister"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command for the `rad credential` command.
//

// NewCommand creates a new command that allows users to manage cloud provider credentials for a Radius installation,
// such as registering, unregistering, listing, and showing credentials.
func NewCommand(factory framework.Factory) *cobra.Command {
	// This command is not runnable, and thus has no runner.
	cmd := &cobra.Command{
		Use:   "credential",
		Short: "Manage cloud provider credential for a Radius installation.",
		Long:  "Manage cloud provider credential for a Radius installation." + common.LongDescriptionBlurb,
		Example: `
# List configured cloud providers credential
rad credential list

# Register (Add or Update) cloud provider credential for Azure with service principal authentication
rad credential register azure --client-id <client id> --client-secret <client secret> --tenant-id <tenant id>
# Register (Add or Update) cloud provider credential for AWS with IAM authentication
rad credential register aws --access-key-id <access-key-id> --secret-access-key <secret-access-key>

# Show cloud provider credential details for Azure
rad credential show azure
# Show cloud provider credential details for AWS
rad credential show aws

# Delete Azure cloud provider configuration
rad credential unregister azure
# Delete AWS cloud provider configuration
rad credential unregister aws
`,
	}

	create := credential_register.NewCommand(factory)
	cmd.AddCommand(create)

	delete, _ := credential_unregister.NewCommand(factory)
	cmd.AddCommand(delete)

	list, _ := credential_list.NewCommand(factory)
	cmd.AddCommand(list)

	show, _ := credential_show.NewCommand(factory)
	cmd.AddCommand(show)

	return cmd
}
