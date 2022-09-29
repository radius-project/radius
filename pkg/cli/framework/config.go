// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package framework

import (
	"context"
	"strings"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/viper"
)

type ConfigHolder struct {
	ConfigFilePath string
	Config         *viper.Viper
}

type contextKey string

func NewContextKey(purpose string) contextKey {
	return contextKey("radius context " + purpose)
}

// Fetches radius config from the viper context
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
	EditWorkspaces(ctx context.Context, config *viper.Viper, workspace *workspaces.Workspace, azureProvider *azure.Provider) error
}

type ConfigFileInterfaceImpl struct {
}

// Edits and updates the rad config file with the specified sections to edit
func (i *ConfigFileInterfaceImpl) EditWorkspaces(ctx context.Context, config *viper.Viper, workspace *workspaces.Workspace, azureProvider *azure.Provider) error {
	err := cli.EditWorkspaces(ctx, config, func(section *cli.WorkspaceSection) error {
		// TODO: Add checks for duplicate workspace names and append random number mechanisms
		workspace := workspace

		if azureProvider != nil {
			workspace.ProviderConfig.Azure = &workspaces.AzureProvider{
				SubscriptionID: azureProvider.SubscriptionID,
				ResourceGroup:  azureProvider.ResourceGroup,
			}
		}

		name := strings.ToLower(workspace.Name)
		// Set the default workspace only if it is run for the first time
		// Workspace switch should be used after
		if section.Default == "" {
			section.Default = name
		}
		section.Items[name] = *workspace

		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (i *ConfigFileInterfaceImpl) ConfigFromContext(ctx context.Context) *viper.Viper {
	return ConfigFromContext(ctx)
}
