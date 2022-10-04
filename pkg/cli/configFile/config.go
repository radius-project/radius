// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package configFile

import (
	"context"
	"strings"

	"github.com/project-radius/radius/pkg/cli"
)

//go:generate mockgen -destination=./mock_config.go -package=configFile -self_package github.com/project-radius/radius/pkg/cli/configFile github.com/project-radius/radius/pkg/cli/configFile Interface

type Interface interface {
	EditWorkspaces(ctx context.Context, filePath string, workspaceName string, environmentName string, resourceGroup string) error
}

type Impl struct {
}

// Edits and updates the rad config file with the specified sections to edit
func (i *Impl) EditWorkspaces(ctx context.Context, filePath string, workspaceName string, environmentName string, resourceGroup string) error {
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
