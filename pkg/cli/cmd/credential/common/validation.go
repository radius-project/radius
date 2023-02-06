// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package common

import (
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/spf13/cobra"
)

// Used in tests
const (
	SelectExistingEnvironmentPrompt         = "Select an existing environment or create a new one"
	SelectExistingEnvironmentCreateSentinel = "[create new]"
	EnterEnvironmentNamePrompt              = "Enter an environment name"
	EnterNamespacePrompt                    = "Enter a namespace name to deploy apps into"
	AzureCredentialID                       = "/planes/azure/azurecloud/providers/System.Azure/credentials/%s"
	AWSCredentialID                         = "/planes/aws/aws/providers/System.AWS/credentials/%s"
)

func ValidateCloudProviderName(name string) error {
	if strings.EqualFold(name, "azure") || strings.EqualFold(name, "aws"){
		return nil
	}

	return &cli.FriendlyError{Message: fmt.Sprintf("Cloud provider type %q is not supported. Supported types: azure.", name)}
}

// SelectExistingEnvironment prompts the user to select from existing environments (with the option to create a new one).
// We also expect the the existing environments to be a non-empty list, callers should check that.
//
// If the name returned is empty, it means that that either no environment was found or that the user opted to create a new one.
func SelectExistingEnvironment(cmd *cobra.Command, defaultVal string, interactive bool, prompter prompt.Interface, existing []corerp.EnvironmentResource) (string, error) {
	selectedName, err := cmd.Flags().GetString("environment")
	if err != nil {
		return "", err
	}

	// If the user provided a name, let's use that if possible.
	if selectedName != "" {
		for _, env := range existing {
			if strings.EqualFold(selectedName, *env.Name) {
				return selectedName, nil
			}
		}

		// Returing empty tells the caller to create a new one, or to prompt or fail.
		return "", nil
	}

	if !interactive {
		// If an an environment exists that matches the default then choose that.
		for _, env := range existing {
			if strings.EqualFold(defaultVal, *env.Name) {
				return defaultVal, nil
			}
		}

		// Returing empty tells the caller to prompt or fail.
		return "", nil
	}

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

// Selects the environment flag name from user if interactive or sets it from flags or to the default value otherwise
func SelectEnvironmentName(cmd *cobra.Command, defaultVal string, interactive bool, inputPrompter prompt.Interface) (string, error) {
	var envStr string
	var err error

	envStr, err = cmd.Flags().GetString("environment")
	if err != nil {
		return "", err
	}
	if interactive && envStr == "" {
		envStr, err = inputPrompter.GetTextInput(EnterEnvironmentNamePrompt, defaultVal)
		if err != nil {
			return "", err
		}
		if envStr == "" {
			return defaultVal, nil
		}
	} else {
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
