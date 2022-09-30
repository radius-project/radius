// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package configFile

import (
	"context"
	"strings"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/workspaces"
)

//go:generate mockgen -destination=./mock_config.go -package=configFile -self_package github.com/project-radius/radius/pkg/cli/configFile github.com/project-radius/radius/pkg/cli/configFile Interface

type Interface interface {
	EditWorkspacesByName(ctx context.Context, filePath string, workspaceName string, environmentName string) error
	UpdateWorkspaces(ctx context.Context, filePath string, workspace *workspaces.Workspace) error
}

type Impl struct {
}

// Edits and updates the rad config file with the specified sections to edit
func (i *Impl) EditWorkspacesByName(ctx context.Context, filePath string, workspaceName string, environmentName string) error {
	// Reload config so we can see the updates
	config, err := cli.LoadConfig(filePath)
	if err != nil {
		return err
	}

	err = cli.EditWorkspaces(ctx, config, func(section *cli.WorkspaceSection) error {
		ws := section.Items[strings.ToLower(workspaceName)]
		ws.Environment = environmentName
		section.Items[strings.ToLower(workspaceName)] = ws
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

// Updates the rad config file with the specified workspace section
func (i *Impl) UpdateWorkspaces(ctx context.Context, filePath string, workspace *workspaces.Workspace) error {
	// Reload config so we can see the updates
	config, err := cli.LoadConfig(filePath)
	if err != nil {
		return err
	}

	err = cli.EditWorkspaces(ctx, config, func(section *cli.WorkspaceSection) error {
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
