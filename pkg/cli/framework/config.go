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

package framework

import (
	"context"
	"strings"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/aws"
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
// NewContextKey creates a new context key based on the given purpose string.
func NewContextKey(purpose string) contextKey {
	return contextKey("radius context " + purpose)
}

// Fetches radius config from the viper context
//
// # Function Explanation
//
// The ConfigFromContext function retrieves a viper.Viper configuration from a context.Context, and returns nil if the
// configuration is not found.
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
	EditWorkspaces(ctx context.Context, config *viper.Viper, workspace *workspaces.Workspace) error
}

var _ ConfigFileInterface = (*ConfigFileInterfaceImpl)(nil)

type ConfigFileInterfaceImpl struct {
}

// # Function Explanation
//
// SetDefaultWorkspace edits the configuration file to set the default workspace to the given name, and returns an error
// if the operation fails.
func (i *ConfigFileInterfaceImpl) SetDefaultWorkspace(ctx context.Context, config *viper.Viper, name string) error {
	return cli.EditWorkspaces(ctx, config, func(section *cli.WorkspaceSection) error {
		section.Default = name
		return nil
	})
}

// # Function Explanation
//
// DeleteWorkspace deletes a workspace from the configuration file and sets the default workspace to an empty string if
// the deleted workspace was the default workspace. It returns an error if the workspace could not be deleted.
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
// EditWorkspaces adds a workspace to a configuration file, ensuring that the workspace name is lowercase and
// that there are no duplicate workspace names.
func (i *ConfigFileInterfaceImpl) EditWorkspaces(ctx context.Context, config *viper.Viper, workspace *workspaces.Workspace) error {
	err := cli.EditWorkspaces(ctx, config, func(section *cli.WorkspaceSection) error {
		// TODO: Add checks for duplicate workspace names and append random number mechanisms
		workspace := workspace
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
// ConfigFromContext takes in a context object and returns a viper object, or an error if the context object is invalid.
func (i *ConfigFileInterfaceImpl) ConfigFromContext(ctx context.Context) *viper.Viper {
	return ConfigFromContext(ctx)
}
