// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package setup

import (
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/spf13/viper"
)

// # Function Explanation
// 
//	ValidateWorkspaceUniqueness checks if a workspace already exists in the config and returns an error if it does, unless 
//	the overwrite flag is set. It also provides a helpful message to the caller if the workspace already exists.
func ValidateWorkspaceUniqueness(config *viper.Viper, overwrite bool) func(string) (bool, string, error) {
	return func(input string) (bool, string, error) {
		if overwrite {
			return true, "", nil // We're overwriting, so don't bother checking.
		}

		found, err := cli.HasWorkspace(config, input)
		if err != nil {
			return false, "", err
		} else if found {
			return false, fmt.Sprintf("the workspace %q already exists. Specify '--force' to overwrite", input), nil
		}

		return true, "", nil
	}
}
