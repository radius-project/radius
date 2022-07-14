// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package stages

import (
	"context"
	"io"

	"github.com/project-radius/radius/pkg/cli/builders"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/radyaml"
	"github.com/project-radius/radius/pkg/cli/workspaces"
)

type Options struct {
	Workspace         workspaces.Workspace
	ConnectionFactory connections.Factory
	BaseDirectory     string
	Manifest          radyaml.Manifest
	FinalStage        string
	Profile           string
	Stdout            io.Writer
	Stderr            io.Writer
	Builders          map[string]builders.Builder
	Parameters        clients.DeploymentParameters

	// BicepBuildFunc supports overriding the build build process for testing.
	BicepBuildFunc func(ctx context.Context, deployFile string) (map[string]interface{}, error)
}

type processor struct {
	Options
	Parameters   clients.DeploymentParameters
	Results      []StageResult
	CurrentStage stageInfo
}

type stageInfo struct {
	Name         string
	DisplayIndex int
	TotalCount   int
}

// Result captures the results of processing for diagnostic logging and testing.
type StageResult struct {
	Stage            *radyaml.Stage
	Input            clients.DeploymentParameters
	Output           clients.DeploymentParameters
	DeploymentResult *clients.DeploymentResult
}
