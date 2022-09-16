// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package common

import (
	"context"
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func ValidateCloudProviderName(name string) error {
	if strings.EqualFold(name, "azure") {
		return nil
	}

	return &cli.FriendlyError{Message: fmt.Sprintf("Cloud provider type %q is not supported. Supported types: azure.", name)}
}

func SelectEnvironmentName(cmd *cobra.Command, defaultVal string, interactive bool) (string, error) {
	var envStr string
	var err error

	envStr, err = cmd.Flags().GetString("environment")
	if err != nil {
		return "", err
	}
	if interactive && envStr == "" {
		promptMsg := fmt.Sprintf("Enter an environment name [%s]:", defaultVal)
		envStr, err = prompt.TextWithDefault(promptMsg, &defaultVal, prompt.ResourceName)
		if err != nil {
			return "", err
		}
		fmt.Printf("Using %s as environment name\n", envStr)
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

func SelectNamespace(cmd *cobra.Command, defaultVal string, interactive bool) (string, error) {
	var val string
	var err error
	if interactive {
		promptMsg := fmt.Sprintf("Enter a namespace name to deploy apps into [%s]:", defaultVal)
		val, err = prompt.TextWithDefault(promptMsg, &defaultVal, prompt.EmptyValidator)
		if err != nil {
			return "", err
		}
		fmt.Printf("Using %s as namespace name\n", val)
	} else {
		val, _ = cmd.Flags().GetString("namespace")
		if val == "" {
			output.LogInfo("No namespace name provided, using: %v", defaultVal)
			val = defaultVal
		}
	}
	return val, nil
}

type contextKey string

func NewContextKey(purpose string) contextKey {
	return contextKey("radius context " + purpose)
}

func ConfigFromContext(ctx context.Context) *viper.Viper {
	holder := ctx.Value(NewContextKey("config")).(*framework.ConfigHolder)
	if holder == nil {
		return nil
	}

	return holder.Config
}
