/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package group

import (
	group_create "github.com/project-radius/radius/pkg/cli/cmd/group/create"
	group_delete "github.com/project-radius/radius/pkg/cli/cmd/group/delete"
	group_switch "github.com/project-radius/radius/pkg/cli/cmd/group/groupswitch"
	group_list "github.com/project-radius/radius/pkg/cli/cmd/group/list"
	group_show "github.com/project-radius/radius/pkg/cli/cmd/group/show"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad group` command.
func NewCommand(factory framework.Factory) *cobra.Command {
	// This command is not runnable, and thus has no runner.
	cmd := &cobra.Command{
		Use:   "group",
		Short: "Manage resource groups",
		Long: `Manage resource groups
		
Resource groups are used to organize and manage Radius resources. They often contain resources that share a common lifecycle or unit of deployment.

A Radius application and its resources can span one or more resource groups, and do not have to be in the same resource group as the Radius environment into which it's being deployed into.

Note that these resource groups are separate from the Azure cloud provider and Azure resource groups configured with the cloud provider.
`,
		Example: `
# List resource groups in default workspace
rad group list

# Create resource group in specified workspace
rad group create prod -w localWorkspace

# Delete resource group in default workspace
rad group delete prod

# Show details of resource group in default workspace
rad group show dev
`,
	}

	create, _ := group_create.NewCommand(factory)
	cmd.AddCommand(create)

	delete, _ := group_delete.NewCommand(factory)
	cmd.AddCommand(delete)

	list, _ := group_list.NewCommand(factory)
	cmd.AddCommand(list)

	show, _ := group_show.NewCommand(factory)
	cmd.AddCommand(show)

	groupswitch, _ := group_switch.NewCommand(factory)
	cmd.AddCommand(groupswitch)

	return cmd

}
