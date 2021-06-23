// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/rad/clients"
)

const (
	KindAzureCloud                   = "azure"
	KindKubernetes                   = "kubernetes"
	KindLocalRP                      = "localrp"
	EnvironmentKeyDefaultApplication = "defaultapplication"
)

type Environment interface {
	GetName() string
	GetKind() string
	GetDefaultApplication() string

	// GetStatusLink provides an optional URL for display of the environment.
	GetStatusLink() string
}

type DeploymentEnvironment interface {
	CreateDeploymentClient(ctx context.Context) (clients.DeploymentClient, error)
}

func CreateDeploymentClient(ctx context.Context, env Environment) (clients.DeploymentClient, error) {
	de, ok := env.(DeploymentEnvironment)
	if !ok {
		return nil, fmt.Errorf("an environment of kind '%s' does not support deployment", env.GetKind())
	}

	return de.CreateDeploymentClient(ctx)
}

type DiagnosticsEnvironment interface {
	CreateDiagnosticsClient(ctx context.Context) (clients.DiagnosticsClient, error)
}

func CreateDiagnosticsClient(ctx context.Context, env Environment) (clients.DiagnosticsClient, error) {
	de, ok := env.(DiagnosticsEnvironment)
	if !ok {
		return nil, fmt.Errorf("an environment of kind '%s' does not support diagnostics operations", env.GetKind())
	}

	return de.CreateDiagnosticsClient(ctx)
}

type ManagementEnvironment interface {
	CreateManagementClient(ctx context.Context) (clients.ManagementClient, error)
}

func CreateManagementClient(ctx context.Context, env Environment) (clients.ManagementClient, error) {
	me, ok := env.(ManagementEnvironment)
	if !ok {
		return nil, fmt.Errorf("an environment of kind '%s' does not support management operations", env.GetKind())
	}

	return me.CreateManagementClient(ctx)
}
