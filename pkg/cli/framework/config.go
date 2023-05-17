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
	SetDefaultWorkspace(ctx context.Context, config *viper.Viper, name string) error
	DeleteWorkspace(ctx context.Context, config *viper.Viper, name string) error
	EditWorkspaces(ctx context.Context, config *viper.Viper, workspace *workspaces.Workspace) error
}

var _ ConfigFileInterface = (*ConfigFileInterfaceImpl)(nil)

type ConfigFileInterfaceImpl struct {
}

func (i *ConfigFileInterfaceImpl) SetDefaultWorkspace(ctx context.Context, config *viper.Viper, name string) error {
	return cli.EditWorkspaces(ctx, config, func(section *cli.WorkspaceSection) error {
		section.Default = name
		return nil
	})
}

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

func (i *ConfigFileInterfaceImpl) ConfigFromContext(ctx context.Context) *viper.Viper {
	return ConfigFromContext(ctx)
}
