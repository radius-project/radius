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

package radinit

import (
	"strings"

	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/spf13/cobra"
)

const (
	SelectExistingEnvironmentPrompt         = "Select an existing environment or create a new one"
	SelectExistingEnvironmentCreateSentinel = "[create new]"
	EnterNamespacePrompt                    = "Enter a namespace name to deploy apps into"
)

// SelectExistingEnvironment prompts the user to select from existing environments (with the option to create a new one).
// We also expect the the existing environments to be a non-empty list, callers should check that.
//
// If the name returned is empty, it means that that either no environment was found or that the user opted to create a new one.
func SelectExistingEnvironment(cmd *cobra.Command, defaultVal string, prompter prompt.Interface, existing []corerp.EnvironmentResource) (string, error) {
	// On this code path, we're going to prompt for input.
	//
	// Build the list of items in the following way:
	//
	// - default environment (if it exists)
	// - (all other existing environments)
	// - [create new]
	items := []string{}
	for _, env := range existing {
		if strings.EqualFold(defaultVal, *env.Name) {
			items = append(items, defaultVal)
			break
		}
	}
	for _, env := range existing {
		// The default is already in the list
		if !strings.EqualFold(defaultVal, *env.Name) {
			items = append(items, *env.Name)
		}
	}
	items = append(items, SelectExistingEnvironmentCreateSentinel)

	choice, err := prompter.GetListInput(items, SelectExistingEnvironmentPrompt)
	if err != nil {
		return "", err
	}

	if choice == SelectExistingEnvironmentCreateSentinel {
		// Returing empty tells the caller to create a new one.
		return "", nil
	}

	return choice, nil
}

// Gets the namespace value from the user if interactive, otherwise sets it to the namespace flag or default value
func SelectNamespace(cmd *cobra.Command, defaultVal string, interactive bool, prompter prompt.Interface) (string, error) {
	var val string
	var err error
	if interactive {
		val, err = prompter.GetTextInput(EnterNamespacePrompt, defaultVal)
		if err != nil {
			return "", err
		}
		if val == "" {
			return defaultVal, nil
		}
	} else {
		val, _ = cmd.Flags().GetString("namespace")
		if val == "" {
			output.LogInfo("No namespace name provided, using: %v", defaultVal)
			val = defaultVal
		}
	}
	return val, nil
}
