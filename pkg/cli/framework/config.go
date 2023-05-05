// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package framework

import (
	"context"
	"strings"

	"github.com/project-radius/radius/pkg/cli"
	aws "github.com/project-radius/radius/pkg/cli/aws"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/config"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/viper"
)

type ConfigHolder struct {
	ConfigFilePath  string
	Config          *viper.Viper
	DirectoryConfig *config.DirectoryConfig
}

type contextKey string

// # Function Explanation
// 
//	NewContextKey creates a new context key with a given purpose, which can be used to store and retrieve values from a 
//	context. If an invalid purpose is provided, an error is returned.
func NewContextKey(purpose string) contextKey {
	return contextKey("radius context " + purpose)
}

// Fetches radius config from the viper context
//
// # Function Explanation
// 
//	ConfigFromContext retrieves a viper.Viper configuration from the context, returning nil if the configuration is not 
//	present. If an error occurs, the function will panic.
func ConfigFromContext(ctx context.Context) *viper.Viper {
	holder := ctx.Value(NewContextKey("config")).(*ConfigHolder)
	if holder == nil {
		return nil
	}

	return holder.Config
}

//go:generate mockgen -destination=./mock_config.go -package=framework -self_package github.com/project-radius/radius/pkg/cli/framework github.com/project-radius/radius/pkg/cli/framework ConfigFileInterface

type ConfigFileInterface interface {
	ConfigFromContext(ctx context.Context) *viper.Viper
	SetDefaultWorkspace(ctx context.Context, config *viper.Viper, name string) error
	DeleteWorkspace(ctx context.Context, config *viper.Viper, name string) error
	EditWorkspaces(ctx context.Context, config *viper.Viper, workspace *workspaces.Workspace, providersList []any) error
}

var _ ConfigFileInterface = (*ConfigFileInterfaceImpl)(nil)

type ConfigFileInterfaceImpl struct {
}

// # Function Explanation
// 
//	ConfigFileInterfaceImpl.SetDefaultWorkspace edits the workspace section of a given config file, setting the default 
//	workspace to the given name. It returns an error if the edit fails.
func (i *ConfigFileInterfaceImpl) SetDefaultWorkspace(ctx context.Context, config *viper.Viper, name string) error {
	return cli.EditWorkspaces(ctx, config, func(section *cli.WorkspaceSection) error {
		section.Default = name
		return nil
	})
}

// # Function Explanation
// 
//	DeleteWorkspace edits the workspace section of the config file, deleting the workspace with the given name and resetting
//	 the default workspace if it was the one being deleted. It returns an error if any part of the operation fails.
func (i *ConfigFileInterfaceImpl) DeleteWorkspace(ctx context.Context, config *viper.Viper, name string) error {
	return cli.EditWorkspaces(ctx, config, func(section *cli.WorkspaceSection) error {
		delete(section.Items, strings.ToLower(name))
		if strings.EqualFold(section.Default, name) {
			section.Default = ""
		}

		return nil
	})
}

// Edits and updates the rad config file with the specified sections to edit
//
// # Function Explanation
// 
//	The EditWorkspaces function edits the workspace configuration in the config file, by adding the workspace and its 
//	associated providers to the configuration. It returns an error if any issue occurs while editing the configuration.
func (i *ConfigFileInterfaceImpl) EditWorkspaces(ctx context.Context, config *viper.Viper, workspace *workspaces.Workspace, providersList []any) error {
	err := cli.EditWorkspaces(ctx, config, func(section *cli.WorkspaceSection) error {
		// TODO: Add checks for duplicate workspace names and append random number mechanisms
		workspace := workspace

		populateProvidersToWorkspace(workspace, providersList)

		name := strings.ToLower(workspace.Name)
		section.Default = name
		section.Items[name] = *workspace

		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func populateProvidersToWorkspace(workspace *workspaces.Workspace, providersList []any) {
	for _, provider := range providersList {
		switch p := provider.(type) {
		case *azure.Provider:
			if p != nil {
				workspace.ProviderConfig.Azure = &workspaces.AzureProvider{
					SubscriptionID: p.SubscriptionID,
					ResourceGroup:  p.ResourceGroup,
				}
			}
		case *aws.Provider:
			if p != nil {
				workspace.ProviderConfig.AWS = &workspaces.AWSProvider{
					Region:    p.TargetRegion,
					AccountId: p.AccountId,
				}
			}
		}
	}
}

// # Function Explanation
// 
//	ConfigFileInterfaceImpl's ConfigFromContext function retrieves the configuration from the context and returns it as a 
//	Viper object. If the configuration is not found, an error is returned.
func (i *ConfigFileInterfaceImpl) ConfigFromContext(ctx context.Context) *viper.Viper {
	return ConfigFromContext(ctx)
}
