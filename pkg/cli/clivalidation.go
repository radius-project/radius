// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cli

import (
	"fmt"
	"path"

	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type AzureResource struct {
	Name           string
	ResourceType   string
	ResourceGroup  string
	SubscriptionID string
}

// Used by commands that require a named environment to be an azure cloud environment.
func ValidateNamedEnvironment(config *viper.Viper, name string) (environments.Environment, error) {
	env, err := ReadEnvironmentSection(config)
	if err != nil {
		return nil, err
	}

	e, err := env.GetEnvironment(name)
	if err != nil {
		return nil, err
	}

	return e, nil
}

func RequireEnvironment(cmd *cobra.Command, config *viper.Viper) (environments.Environment, error) {
	environmentName, err := cmd.Flags().GetString("environment")
	if err != nil {
		return nil, err
	}

	env, err := ValidateNamedEnvironment(config, environmentName)
	return env, err
}

func RequireEnvironmentArgs(cmd *cobra.Command, config *viper.Viper, args []string) (environments.Environment, error) {
	environmentName, err := RequireEnvironmentNameArgs(cmd, args)
	if err != nil {
		return nil, err
	}

	env, err := ValidateNamedEnvironment(config, environmentName)
	return env, err
}

func RequireEnvironmentNameArgs(cmd *cobra.Command, args []string) (string, error) {
	environmentName, err := cmd.Flags().GetString("environment")
	if err != nil {
		return "", err
	}

	if len(args) > 0 {
		if environmentName != "" {
			return "", fmt.Errorf("cannot specify environment name via both arguments and `-e`")
		}
		environmentName = args[0]
	}

	return environmentName, err
}

func RequireApplicationArgs(cmd *cobra.Command, args []string, env environments.Environment) (string, error) {
	applicationName, err := cmd.Flags().GetString("application")
	if err != nil {
		return "", err
	}

	if len(args) > 0 {
		if args[0] != "" {
			if applicationName != "" {
				return "", fmt.Errorf("cannot specify application name via both arguments and `-a`")
			}
			applicationName = args[0]
		}
	}

	if applicationName == "" {
		applicationName = env.GetDefaultApplication()
		if applicationName == "" {
			return "", fmt.Errorf("no application name provided and no default application set, " +
				"either pass in an application name or set a default application by using `rad application switch`")
		}
	}

	return applicationName, nil
}

func RequireApplication(cmd *cobra.Command, env environments.Environment) (string, error) {
	return RequireApplicationArgs(cmd, []string{}, env)
}

func RequireResource(cmd *cobra.Command, args []string) (resourceType string, resourceName string, err error) {
	results, err := requiredMultiple(cmd, args, "type", "resource")
	if err != nil {
		return "", "", err
	}
	return results[0], results[1], nil
}

func RequireAzureResource(cmd *cobra.Command, args []string) (azureResource AzureResource, err error) {
	results, err := requiredMultiple(cmd, args, "type", "resource", "resource-group", "resource-subscription-id")
	if err != nil {
		return AzureResource{}, err
	}
	return AzureResource{
		Name:           results[0],
		ResourceType:   results[1],
		ResourceGroup:  results[2],
		SubscriptionID: results[3],
	}, nil
}

func RequireOutput(cmd *cobra.Command) (string, error) {
	return cmd.Flags().GetString("output")
}

func RequireRadYAML(cmd *cobra.Command) (string, error) {
	radFile, err := cmd.Flags().GetString("radfile")
	if err != nil {
		return "", err
	}

	if radFile == "" {
		return path.Join(".", "rad.yaml"), nil
	}

	return radFile, nil
}

func requiredMultiple(cmd *cobra.Command, args []string, names ...string) ([]string, error) {
	results := make([]string, len(names))
	for i, name := range names {
		value, err := cmd.Flags().GetString(name)
		if err == nil {
			results[i] = value
		}
		if results[i] != "" {
			if len(args) > len(names)-i-1 {
				return nil, fmt.Errorf("cannot specify %v name via both arguments and switch", name)
			}
			continue
		}
		if len(args) == 0 {
			return nil, fmt.Errorf("no %v name provided", name)
		}
		results[i] = args[0]
		args = args[1:]
	}
	return results, nil
}
