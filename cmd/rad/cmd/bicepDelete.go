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

package cmd

import (
	"github.com/project-radius/radius/pkg/cli/bicep"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

var bicepDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete installed bicep compiler",
	Long:  `Removes the local copy of the bicep compiler`,
	RunE: func(cmd *cobra.Command, args []string) error {
		output.LogInfo("removing local copy of bicep...")
		ok, err := bicep.IsBicepInstalled()
		if err != nil {
			return err
		}

		if !ok {
			output.LogInfo("bicep is not installed")
			return err
		}

		err = bicep.DeleteBicep()
		return err
	},
}

func init() {
	bicepCmd.AddCommand(bicepDeleteCmd)
}
