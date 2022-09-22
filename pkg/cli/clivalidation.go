// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli/ucp"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
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

func RequireEnvironmentNameArgs(cmd *cobra.Command, args []string, workspace workspaces.Workspace) (string, error) {
	environmentName, err := ReadEnvironmentNameArgs(cmd, args)
	if err != nil {
		return "", err
	}

	// We store the environment id in config, but most commands work with the environment name.
	if environmentName == "" && workspace.Environment != "" {
		id, err := resources.Parse(workspace.Environment)
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
		id, err := resources.Parse(workspace.Environment)
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

func RequireApplicationArgs(cmd *cobra.Command, args []string, workspace workspaces.Workspace) (string, error) {
	applicationName, err := ReadApplicationNameArgs(cmd, args)
	if err != nil {
		return "", err
	}

	if applicationName == "" {
		applicationName = workspace.DefaultApplication
	}

	if applicationName == "" {
		return "", fmt.Errorf("no application name provided and no default application set, " +
			"either pass in an application name or set a default application by using `rad application switch`")
	}

	return applicationName, nil
}

func ReadApplicationName(cmd *cobra.Command, workspace workspaces.Workspace) (string, error) {
	applicationName, err := cmd.Flags().GetString("application")
	if err != nil {
		return "", err
	}

	if applicationName == "" {
		applicationName = workspace.DefaultApplication
	}

	return applicationName, nil
}

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

// example of resource Type: Applications.Core/httpRoutes, Applications.Connector/redisCaches
func RequireResourceType(args []string) (string, error) {
	if len(args) < 1 {
		return "", errors.New("no resource type provided")
	}
	resourceTypeName := args[0]
	supportedTypes := []string{}
	for _, resourceType := range ucp.ResourceTypesList {
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

func RequireOutput(cmd *cobra.Command) (string, error) {
	return cmd.Flags().GetString("output")
}

// RequireWorkspace is used by commands that require an existing workspace either set as the default,
// or specified using the 'workspace' flag.
func RequireWorkspace(cmd *cobra.Command, config *viper.Viper) (*workspaces.Workspace, error) {
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

// Is404Error returns true if the error is a 404 payload from an autorest operation.
func Is404ErrorForAzureError(err error) bool {
	if err == nil {
		return false
	}

	// The error might already be an ResponseError
	responseError := &azcore.ResponseError{}
	if errors.As(err, &responseError) && responseError.ErrorCode == v1.CodeNotFound {
		return true
	} else if errors.As(err, &responseError) {
		return false
	}

	// OK so it's not an ResponseError, can we turn it into an ErrorResponse?
	errorResponse := v20220315privatepreview.ErrorResponse{}
	marshallErr := json.Unmarshal([]byte(err.Error()), &errorResponse)
	if marshallErr != nil {
		return false
	}

	if errorResponse.Error != nil && *errorResponse.Error.Code == v1.CodeNotFound {
		return true
	}

	return false
}
