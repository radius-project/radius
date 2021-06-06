// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clients

import (
	"context"
)

// DeploymentClient is used to deploy ARM-JSON templates (compiled Bicep output).
type DeploymentClient interface {
	Deploy(ctx context.Context, content string) error
}

// DiagnosticsClient is used to interface with diagnostics features like logs and port-forwards.
type DiagnosticsClient interface {
	Expose(ctx context.Context, options ExposeOptions) error
	Logs(ctx context.Context, options LogsOptions) error
}

type ExposeOptions struct {
	Application string
	Component   string
	Port        int
	RemotePort  int
}

type LogsOptions struct {
	Application string
	Component   string
}

// ManagementClient is used to interface with management features like listing applications and components.
type ManagementClient interface {
}
