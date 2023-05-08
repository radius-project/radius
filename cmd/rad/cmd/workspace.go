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

package cmd

import (
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(workspaceCmd)
	workspaceCmd.PersistentFlags().StringP("workspace", "w", "", "The workspace name")
}

func NewWorkspaceCommand() *cobra.Command {
	// This command is not runnable, and thus has no runner.
	return &cobra.Command{
		Use:   "workspace",
		Short: "Manage workspaces",
		Long: `Manage workspaces
		Workspaces allow you to manage multiple Radius platforms and environments using a local configuration file. 
		You can easily define and switch between workspaces to deploy and manage applications across local, test, and production environments.
		`,
		Example: `
# Create workspace with no default resource group or environment set
rad workspace create kubernetes myworkspace --context kind-kind
# Create workspace with default resource group and environment set
rad workspace create kubernetes myworkspace --context kind-kind --group myrg --environment myenv
`,
	}
}
