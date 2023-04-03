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
	"github.com/spf13/cobra"
)

// Used in tests
const (
	AzureCloudProvider                      = "Azure"
	AWSCloudProvider                        = "AWS"
	SelectExistingEnvironmentPrompt         = "Select an existing environment or create a new one"
	SelectExistingEnvironmentCreateSentinel = "[create new]"
	EnterEnvironmentNamePrompt              = "Enter an environment name"
	EnterNamespacePrompt                    = "Enter a namespace name to deploy apps into"
	AzureCredentialID                       = "/planes/azure/azurecloud/providers/System.Azure/credentials/%s"
	AWSCredentialID                         = "/planes/aws/aws/providers/System.AWS/credentials/%s"
)

var (
	supportedProviders = []string{AzureCloudProvider, AWSCloudProvider}
)

func ValidateCloudProviderName(name string) error {
	for _, provider := range supportedProviders {
		if strings.EqualFold(name, provider) {
			return nil
		}
	}

	return &cli.FriendlyError{Message: fmt.Sprintf("Cloud provider type %q is not supported. ", strings.Join(supportedProviders, " "))}
}

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
