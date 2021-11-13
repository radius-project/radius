// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package stages

import (
	"context"

	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/Azure/radius/pkg/cli/environments"
	"github.com/Azure/radius/pkg/cli/radyaml"
)

type Options struct {
	Environment   environments.Environment
	BaseDirectory string
	Manifest      radyaml.Manifest
	FinalStage    string
	Parameters    clients.DeploymentParameters

	// BicepBuildFunc supports overriding the build build process for testing.
	BicepBuildFunc func(ctx context.Context, deployFile string) (string, error)
}

type processor struct {
	Options
	Parameters clients.DeploymentParameters
	Results    []StageResult
}

// Result captures the results of processing for diagnostic logging and testing.
type StageResult struct {
	Stage            *radyaml.Stage
	Input            clients.DeploymentParameters
	Output           clients.DeploymentParameters
	DeploymentResult *clients.DeploymentResult
}
