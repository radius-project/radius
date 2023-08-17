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

package create

import (
	"fmt"

	"github.com/spf13/cobra"
)

// ValidateArgs returns an error if the args .
//

// ValidateArgs checks if the number of arguments passed to the command is between 1 and 2, and if the first argument is
// "kubernetes", and returns an error if either of these conditions are not met.
func ValidateArgs() cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 || len(args) > 2 {
			return fmt.Errorf("usage: rad workspace create [workspaceType] [workspaceName] [flags]")
		}
		if args[0] != "kubernetes" {
			return fmt.Errorf("workspaces currently only support type 'kubernetes'")
		}
		return nil
	}
}
