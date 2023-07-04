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

package cli

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/clierrors"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/config"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type AzureResource struct {
	Name           string
	ResourceType   string
	ResourceGroup  string
	SubscriptionID string
}

const (
	LinkTypeFlag = "link-type"
)

// # Function Explanation
//
// RequireEnvironmentNameArgs checks if an environment name is provided as an argument or if a default environment is set
// in the workspace, and returns an error if neither is the case. It also handles any errors that may occur while parsing
// the environment resource.
func RequireEnvironmentNameArgs(cmd *cobra.Command, args []string, workspace workspaces.Workspace) (string, error) {
	environmentName, err := ReadEnvironmentNameArgs(cmd, args)
	if err != nil {
		return "", err
	}

	// We store the environment id in config, but most commands work with the environment name.
	if environmentName == "" && workspace.Environment != "" {
		id, err := resources.ParseResource(workspace.Environment)
		if err != nil {
			return "", err
		}

		environmentName = id.Name()
	}

	if environmentName == "" {
		return "", fmt.Errorf("no environment name provided and no default environment set, " +
			"either pass in an environment name or set a default environment by using `rad env switch`")
	}

	return environmentName, err
}

// # Function Explanation
//
// RequireEnvironmentName checks if an environment name is provided as a flag or as a default environment in the workspace,
// and returns an error if neither is present. It also handles any errors that occur while parsing the resource.
func RequireEnvironmentName(cmd *cobra.Command, args []string, workspace workspaces.Workspace) (string, error) {
	environmentName, err := cmd.Flags().GetString("environment")
	if err != nil {
		return "", err
	}

	// We store the environment id in config, but most commands work with the environment name.
	if environmentName == "" && workspace.Environment != "" {
		id, err := resources.ParseResource(workspace.Environment)
		if err != nil {
			return "", err
		}

		environmentName = id.Name()
	}

	if environmentName == "" && workspace.IsEditableWorkspace() {
		// Setting a default environment only applies to editable workspaces
		return "", fmt.Errorf("no environment name provided and no default environment set, " +
			"either pass in an environment name or set a default environment by using `rad env switch`")
	} else if environmentName == "" {
		return "", fmt.Errorf("no environment name provided, pass in an environment name")
	}

	return environmentName, err
}

// RequireKubeContext is used by commands that need a kubernetes context name to be specified using -c flag or has a default kubecontext
//
// # Function Explanation
//
// RequireKubeContext checks if a kubecontext is provided as a flag, and if not, uses the current context. If neither is
// provided, it returns an error.
func RequireKubeContext(cmd *cobra.Command, currentContext string) (string, error) {
	kubecontext, err := cmd.Flags().GetString("context")
	if err != nil {
		return "", err
	}

	if kubecontext == "" && currentContext == "" {
		return "", errors.New("the kubeconfig has no current context")
	} else if kubecontext == "" {
		kubecontext = currentContext
	}

	return kubecontext, nil
}

// # Function Explanation
//
// ReadEnvironmentNameArgs reads the environment name from either the command line arguments or the "-e" flag, and returns
// an error if both are specified.
func ReadEnvironmentNameArgs(cmd *cobra.Command, args []string) (string, error) {
	name, err := cmd.Flags().GetString("environment")
	if err != nil {
		return "", err
	}

	if len(args) > 0 {
		if name != "" {
			return "", fmt.Errorf("cannot specify environment name via both arguments and `-e`")
		}
		name = args[0]
	}

	return name, err
}

// RequireApplicationArgs reads the application name from the following sources in priority order and returns
// an error if no application name is set.
//
// - '--application' flag
// - first positional arg
// - workspace default application
// - directory config application
//
// # Function Explanation
//
// RequireApplicationArgs checks if an application name is provided as an argument, and if not, checks if a default
// application is set in the workspace. If no application name is provided, it returns an error.
func RequireApplicationArgs(cmd *cobra.Command, args []string, workspace workspaces.Workspace) (string, error) {
	applicationName, err := ReadApplicationNameArgs(cmd, args)
	if err != nil {
		return "", err
	}

	if applicationName == "" {
		applicationName = workspace.DefaultApplication
	}

	if applicationName == "" {
		applicationName = workspace.DirectoryConfig.Workspace.Application
	}

	if applicationName == "" {
		return "", fmt.Errorf("no application name provided and no default application set, " +
			"either pass in an application name or set a default application by using `rad application switch`")
	}

	return applicationName, nil
}

// ReadApplicationName reads the application name from the following sources in priority order and returns
// the empty string if no application is set.
//
// - '--application' flag
// - workspace default application
// - directory config application
//
// # Function Explanation
//
// ReadApplicationName reads the application name from the command line flag and, if not provided, from the workspace
// configuration. It returns an error if the flag is not set correctly.
func ReadApplicationName(cmd *cobra.Command, workspace workspaces.Workspace) (string, error) {
	applicationName, err := cmd.Flags().GetString("application")
	if err != nil {
		return "", err
	}

	if applicationName == "" {
		applicationName = workspace.DefaultApplication
	}

	if applicationName == "" {
		applicationName = workspace.DirectoryConfig.Workspace.Application
	}

	return applicationName, nil
}

// ReadApplicationName reads the application name from the following sources in priority order and returns
// the empty string if no application is set.
//
// - '--application' flag
// - first positional arg
//
// # Function Explanation
//
// ReadApplicationNameArgs reads the application name from either the command line arguments or the "-a" flag, and returns
// an error if both are specified.
func ReadApplicationNameArgs(cmd *cobra.Command, args []string) (string, error) {
	name, err := cmd.Flags().GetString("application")
	if err != nil {
		return "", err
	}

	if len(args) > 0 {
		if name != "" {
			return "", fmt.Errorf("cannot specify application name via both arguments and `-a`")
		}
		name = args[0]
	}

	return name, err
}

// RequireApplicationArgs reads the application name from the following sources in priority order and returns
// an error if no application name is set.
//
// - '--application' flag
// - workspace default application
// - directory config application
//
// # Function Explanation
//
// RequireApplication requires the user to provide an application name as an argument and returns it as a string. If the
// user does not provide an application name, an error is returned.
func RequireApplication(cmd *cobra.Command, workspace workspaces.Workspace) (string, error) {
	return RequireApplicationArgs(cmd, []string{}, workspace)
}

// # Function Explanation
//
// RequireResource parses the given command and arguments to extract two required values, a resource type and a resource
// name, and returns them. If either of the values is missing, an error is returned.
func RequireResource(cmd *cobra.Command, args []string) (resourceType string, resourceName string, err error) {
	results, err := requiredMultiple(cmd, args, "type", "resource")
	if err != nil {
		return "", "", err
	}
	return results[0], results[1], nil
}

// # Function Explanation
//
// RequireResourceTypeAndName checks if the provided arguments contain a resource type and name, and returns them if they
// are present. If either is missing, an error is returned.
func RequireResourceTypeAndName(args []string) (string, string, error) {
	if len(args) < 2 {
		return "", "", errors.New("No resource type or name provided")
	}
	resourceType, err := RequireResourceType(args)
	if err != nil {
		return "", "", err
	}
	resourceName := args[1]
	return resourceType, resourceName, nil
}

// example of resource Type: Applications.Core/httpRoutes, Applications.Link/redisCaches
//
// # Function Explanation
//
// RequireResourceType checks if the first argument provided is a valid resource type and returns it if it is. If the
// argument is not valid, an error is returned with a list of valid resource types.
func RequireResourceType(args []string) (string, error) {
	if len(args) < 1 {
		return "", errors.New("no resource type provided")
	}
	resourceTypeName := args[0]
	supportedTypes := []string{}
	for _, resourceType := range clients.ResourceTypesList {
		supportedType := strings.Split(resourceType, "/")[1]
		supportedTypes = append(supportedTypes, supportedType)
		if strings.EqualFold(supportedType, resourceTypeName) {
			return resourceType, nil
		}
	}
	return "", fmt.Errorf("'%s' is not a valid resource type. Available Types are: \n\n%s\n",
		resourceTypeName, strings.Join(supportedTypes, "\n"))
}

// # Function Explanation
//
// "RequireAzureResource" takes in a command and a slice of strings and returns an AzureResource object and an error. It
// uses the "requiredMultiple" function to get the values of the required parameters and then creates an AzureResource
// object with those values. If any of the required parameters are missing, it returns an error.
func RequireAzureResource(cmd *cobra.Command, args []string) (azureResource AzureResource, err error) {
	results, err := requiredMultiple(cmd, args, "type", "resource", "resource-group", "resource-subscription-id")
	if err != nil {
		return AzureResource{}, err
	}
	return AzureResource{
		ResourceType:   results[0],
		Name:           results[1],
		ResourceGroup:  results[2],
		SubscriptionID: results[3],
	}, nil
}

// RequireAzureSubscriptionId is used by commands that require specifying an Azure subscriptionId using a flag
func RequireAzureSubscriptionId(cmd *cobra.Command) (string, error) {
	subscriptionId, err := cmd.Flags().GetString(commonflags.AzureSubscriptionIdFlag)
	if err != nil {
		return "", err
	}

	// Validate that subscriptionId is a valid GUID
	if _, err := uuid.Parse(subscriptionId); err != nil {
		return "", fmt.Errorf("'%s' is not a valid subscription ID", subscriptionId)
	}

	return subscriptionId, err
}

func RequireOutput(cmd *cobra.Command) (string, error) {
	return cmd.Flags().GetString("output")
}

// RequireWorkspace is used by commands that require an existing workspace either set as the default,
// or specified using the 'workspace' flag.
//
// # Function Explanation
//
// RequireWorkspace reads the workspace name from the command flags, retrieves the workspace from the configuration, and
// returns it, or a fallback workspace if none is found. It also handles any errors that may occur during the process.
func RequireWorkspace(cmd *cobra.Command, config *viper.Viper, dc *config.DirectoryConfig) (*workspaces.Workspace, error) {
	name, err := cmd.Flags().GetString("workspace")
	if err != nil {
		return nil, err
	}

	section, err := ReadWorkspaceSection(config)
	if err != nil {
		return nil, err
	}

	ws, err := section.GetWorkspace(name)
	if err != nil {
		return nil, err
	}

	// If we get here and ws is nil then this means there's no default set (or no config).
	// Lets use the fallback configuration.
	if ws == nil {
		ws = workspaces.MakeFallbackWorkspace()
	}

	if dc != nil {
		ws.DirectoryConfig = *dc
	}

	return ws, nil
}

// RequireUCPResourceGroup is used by commands that require specifying a UCP resouce group name using flag or positional args
//
// # Function Explanation
//
// RequireUCPResourceGroup reads the resource group name from the command line arguments and returns an error if the name
// is not provided or is empty. It also handles any errors that may occur while reading the resource group name.
func RequireUCPResourceGroup(cmd *cobra.Command, args []string) (string, error) {
	group, err := ReadResourceGroupNameArgs(cmd, args)
	if err != nil {
		return "", err
	}
	if group == "" {
		return "", fmt.Errorf("resource group name is not provided or is empty ")
	}

	return group, nil
}

// ReadResourceGroupNameArgs is used to get the resource group name that is supplied as either the first argument for group commands or using a -g flag
//
// # Function Explanation
//
// ReadResourceGroupNameArgs reads a resource group name from either a command flag or an argument, and returns an error if
//
//	both are specified.
func ReadResourceGroupNameArgs(cmd *cobra.Command, args []string) (string, error) {
	name, err := cmd.Flags().GetString("group")
	if err != nil {
		return "", err
	}

	if len(args) > 0 {
		if name != "" {
			return "", fmt.Errorf("cannot specify resource group name via both arguments and `-g`")
		}
		name = args[0]
	}

	return name, err
}

// RequireWorkspaceArgs is used by commands that require an existing workspace either set as the default,
// or specified as a positional arg, or specified using the 'workspace' flag.
//
// # Function Explanation
//
// RequireWorkspaceArgs reads the workspace name from the command line arguments, retrieves the workspace from the
// configuration, and returns it. If the workspace is not found, it returns a fallback workspace. If any errors occur, it
// returns an error.
func RequireWorkspaceArgs(cmd *cobra.Command, config *viper.Viper, args []string) (*workspaces.Workspace, error) {
	name, err := ReadWorkspaceNameArgs(cmd, args)
	if err != nil {
		return nil, err
	}

	section, err := ReadWorkspaceSection(config)
	if err != nil {
		return nil, err
	}

	ws, err := section.GetWorkspace(name)
	if err != nil {
		return nil, err
	}

	// If we get here and ws is nil then this means there's no default set (or no config).
	// Lets use the fallback configuration.
	if ws == nil {
		ws = workspaces.MakeFallbackWorkspace()
	}

	return ws, nil
}

// ReadWorkspaceNameArgs is used to get the workspace name that is supplied as either the first argument or using a -w flag
//
// # Function Explanation
//
// ReadWorkspaceNameArgs reads a workspace name from either a command flag or an argument, and returns an error if both are
//
//	specified.
func ReadWorkspaceNameArgs(cmd *cobra.Command, args []string) (string, error) {
	name, err := cmd.Flags().GetString("workspace")
	if err != nil {
		return "", err
	}

	if len(args) > 0 {
		if name != "" {
			return "", fmt.Errorf("cannot specify workspace name via both arguments and `-w`")
		}
		name = args[0]
	}

	return name, err
}

// ReadWorkspaceName is used to get the workspace name that is supplied using a -w flag or as second arg.
//
// # Function Explanation
//
// ReadWorkspaceNameSecondArg checks if a workspace name is provided via a flag or as the second argument in the command,
// and returns an error if both are specified.
func ReadWorkspaceNameSecondArg(cmd *cobra.Command, args []string) (string, error) {
	name, err := cmd.Flags().GetString("workspace")
	if err != nil {
		return "", err
	}

	if len(args) > 1 {
		if name != "" {
			return "", fmt.Errorf("cannot specify workspace name via both arguments and `-w`")
		}
		name = args[1]
	}

	return name, err
}

// # Function Explanation
//
// RequireRadYAML checks if a radfile flag is provided and if not, returns the default rad.yaml file in the current
// directory. If an error occurs, it is returned to the caller.
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

// RequireScope returns the scope the command should use to execute or an error if unset.
//
// This function considers the following sources:
//
// - --group flag
// - workspace scope
//
// # Function Explanation
//
// RequireScope checks if a resource group is passed in as a flag and returns the scope of the resource group if it is,
// otherwise it returns the scope of the workspace if it is set, otherwise it returns an error.
func RequireScope(cmd *cobra.Command, workspace workspaces.Workspace) (string, error) {
	resourceGroup, err := cmd.Flags().GetString("group")
	if err != nil {
		return "", err
	}

	if resourceGroup != "" {
		return fmt.Sprintf("/planes/radius/local/resourceGroups/%s", resourceGroup), nil
	} else if workspace.Scope != "" {
		return workspace.Scope, nil
	} else {
		return "", clierrors.Message("No resource group set, use `--group` to pass in a resource group name.")
	}
}

// # Function Explanation
//
// RequireRecipeNameArgs checks if the provided arguments contain at least one string, and if not, returns an error. If the
//
//	arguments contain a string, it is returned.
func RequireRecipeNameArgs(cmd *cobra.Command, args []string) (string, error) {
	if len(args) < 1 {
		return "", errors.New("no recipe name provided")
	}
	return args[0], nil
}

// # Function Explanation
//
// RequireLinkType retrieves the link type flag from the given command and returns it, or an error if the flag is not set.
func RequireLinkType(cmd *cobra.Command) (string, error) {
	linkType, err := cmd.Flags().GetString(LinkTypeFlag)
	if err != nil {
		return linkType, err
	}
	return linkType, nil
}
