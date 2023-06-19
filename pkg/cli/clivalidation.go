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
func RequireApplication(cmd *cobra.Command, workspace workspaces.Workspace) (string, error) {
	return RequireApplicationArgs(cmd, []string{}, workspace)
}

func RequireResource(cmd *cobra.Command, args []string) (resourceType string, resourceName string, err error) {
	results, err := requiredMultiple(cmd, args, "type", "resource")
	if err != nil {
		return "", "", err
	}
	return results[0], results[1], nil
}

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
func RequireAzureSubscriptionId(cmd *cobra.Command, args []string) (string, error) {
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

func RequireRecipeNameArgs(cmd *cobra.Command, args []string) (string, error) {
	if len(args) < 1 {
		return "", errors.New("no recipe name provided")
	}
	return args[0], nil
}

func RequireLinkType(cmd *cobra.Command) (string, error) {
	linkType, err := cmd.Flags().GetString(LinkTypeFlag)
	if err != nil {
		return linkType, err
	}
	return linkType, nil
}
