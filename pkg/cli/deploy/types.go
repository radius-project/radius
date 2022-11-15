// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deploy

import (
	"context"

	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/workspaces"
)

// Interface is the interface for executing Bicep deployments in the CLI.
type Interface interface {
	// DeployWithProgress runs a deployment and displays progress to the user. This is intended to be used
	// from the CLI and thus logs to the console.
	DeployWithProgress(ctx context.Context, options Options) (clients.DeploymentResult, error)
}

// Options contains options to be used with DeployWithProgress.
type Options struct {
	// ConnectionFactory is used to create the deployment client.
	ConnectionFactory connections.Factory

	// Parameters should contain the parameters to set for the deployment.
	Parameters clients.DeploymentParameters

	// Template should contain a parsed ARM-JSON template.
	Template map[string]interface{}

	// ApplicationID is the resource ID of the application. If provided, will be used as configuration for the Radius provider.
	ApplicationID string

	// EnvironmentID is the resource ID of the environment. If provided, will be used as configuration for the Radius provider.
	EnvironmentID string

	// Workspace is the workspace to use for deployment.
	Workspace workspaces.Workspace

	// ProgressText is a message displayed on the console when deployment begins.
	ProgressText string

	// CompleteText is a message displayed on the console when deployment completes.
	CompletionText string
}

var _ Interface = (*Impl)(nil)

type Impl struct {
}

//go:generate mockgen -destination=./mock_deploy.go -package=deploy -self_package github.com/project-radius/radius/pkg/cli/deploy github.com/project-radius/radius/pkg/cli/deploy Interface

// DeployWithProgress runs a deployment and displays progress to the user. This is intended to be used
// from the CLI and thus logs to the console.
func (*Impl) DeployWithProgress(ctx context.Context, options Options) (clients.DeploymentResult, error) {
	return DeployWithProgress(ctx, options)
}
