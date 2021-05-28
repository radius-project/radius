package rad

import (
	"fmt"

	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Used by commands that require a named environment to be an azure cloud environment.
func ValidateNamedEnvironment(name string) (*environments.AzureCloudEnvironment, error) {
	v := viper.GetViper()
	env, err := ReadEnvironmentSection(v)
	if err != nil {
		return nil, err
	}

	e, err := env.GetEnvironment(name)
	if err != nil {
		return nil, err
	}

	return environments.RequireAzureCloud(e)
}

func RequireEnvironment(cmd *cobra.Command) (*environments.AzureCloudEnvironment, error) {
	environmentName, err := cmd.Flags().GetString("environment")
	if err != nil {
		return nil, err
	}

	env, err := ValidateNamedEnvironment(environmentName)
	return env, err
}

func RequireEnvironmentArgs(cmd *cobra.Command, args []string) (*environments.AzureCloudEnvironment, error) {
	environmentName, err := RequireEnvironmentNameArgs(cmd, args)
	if err != nil {
		return nil, err
	}

	env, err := ValidateNamedEnvironment(environmentName)
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

func RequireApplicationArgs(cmd *cobra.Command, args []string, env *environments.AzureCloudEnvironment) (string, error) {
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
				"either pass in an application name or set a default application by using `rad appplication switch`")
		}
	}

	return applicationName, nil
}

func RequireApplication(cmd *cobra.Command, env *environments.AzureCloudEnvironment) (string, error) {
	return RequireApplicationArgs(cmd, []string{}, env)
}

func RequireDeployment(cmd *cobra.Command, args []string) (string, error) {
	return required(cmd, args, "deployment")
}

func RequireComponent(cmd *cobra.Command, args []string) (string, error) {
	return required(cmd, args, "component")
}

func required(cmd *cobra.Command, args []string, name string) (string, error) {
	value, err := cmd.Flags().GetString(name)
	if err != nil {
		return "", err
	}

	if len(args) > 0 {
		if value != "" {
			return "", fmt.Errorf("cannot specify %v name via both arguments and switch", name)
		}
		value = args[0]
	}

	if value == "" {
		return "", fmt.Errorf("no %v name provided", name)
	}

	return value, nil
}
