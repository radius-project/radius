// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package validation

import (
	"fmt"

	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/spf13/cobra"
)

// Used in tests
const (
	EnterEnvironmentNamePrompt = "Enter an environment name"
)

// Selects the environment flag name from user if interactive or sets it from flags or to the default value otherwise
func SelectEnvironmentName(cmd *cobra.Command, defaultVal string, interactive bool, inputPrompter prompt.Interface) (string, error) {
	var envStr string
	var err error

	if interactive {
		envStr, err = inputPrompter.GetTextInput(EnterEnvironmentNamePrompt, defaultVal)
		if err != nil {
			return "", err
		}
		if envStr == "" {
			return defaultVal, nil
		}
	} else {
		envStr, err = cmd.Flags().GetString("environment")
		if err != nil {
			return "", err
		}
		if envStr == "" {
			output.LogInfo("No environment name provided, using: %v", defaultVal)
			envStr = defaultVal
		}
		matched, msg, _ := prompt.ResourceName(envStr)
		if !matched {
			return "", fmt.Errorf("%s %s. Use --environment option to specify the valid name", envStr, msg)
		}
	}

	return envStr, nil
}
