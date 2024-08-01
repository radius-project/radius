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

package aws

import (
	"github.com/radius-project/radius/pkg/cli/cmd/credential/common"
	"github.com/radius-project/radius/pkg/cli/cmd/credential/register/aws/accesskey"
	"github.com/radius-project/radius/pkg/cli/cmd/credential/register/aws/irsa"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad credential register aws` command.
//

// NewCommand creates a new cobra command for registering AWS cloud provider credentials with IAM authentication, and
// returns a Runner to execute the command.
func NewCommand(factory framework.Factory) *cobra.Command {
	// This command is not runnable, and thus has no runner.
	cmd := &cobra.Command{
		Use:   "aws",
		Short: "Register (Add or update) AWS cloud provider credential for a Radius installation.",
		Long:  "Register (Add or update) AWS cloud provider credential for a Radius installation.." + common.LongDescriptionBlurb,
		Example: `
# Register (Add or update) cloud provider credential for AWS with access key authentication.
rad credential register aws access-key --access-key-id <access-key-id> --secret-access-key <secret-access-key>
# Register (Add or update) cloud provider credential for AWS with IRSA.
rad credential register aws irsa --iam-role <roleARN>
`,
	}

	accesskey, _ := accesskey.NewCommand(factory)
	cmd.AddCommand(accesskey)

	irsa, _ := irsa.NewCommand(factory)
	cmd.AddCommand(irsa)

	return cmd
}
