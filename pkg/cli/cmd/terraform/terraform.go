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

package terraform

import (
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the `rad terraform` command.
func NewCommand() *cobra.Command {
	// This command is not runnable, and thus has no runner.
	cmd := &cobra.Command{
		Use:   "terraform",
		Short: "Manage Terraform installation for Radius",
		Long: `Manage Terraform installation for Radius. Terraform is used by Radius to execute Terraform recipes.

Use subcommands to install, uninstall, or check the status of Terraform.`,
	}

	return cmd
}
