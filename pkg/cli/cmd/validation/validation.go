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
	AzureCloudProvider         = "Azure"
	AWSCloudProvider           = "AWS"
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
