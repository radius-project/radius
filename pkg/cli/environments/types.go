// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/cli/clients"
)

const (
	KindAzureCloud                   = "azure"
	KindDev                          = "dev"
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

type Providers struct {
	AzureProvider *AzureProvider `mapstructure:"azure,omitempty"`
}

type AzureProvider struct {
	SubscriptionID string `mapstructure:"subscriptionid" validate:"required"`
	ResourceGroup  string `mapstructure:"resourcegroup" validate:"required"`
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

type ServerLifecycleEnvironment interface {
	CreateServerLifecycleClient(ctx context.Context) (clients.ServerLifecycleClient, error)
}

func CreateServerLifecycleClient(ctx context.Context, env Environment) (clients.ServerLifecycleClient, error) {
	me, ok := env.(ServerLifecycleEnvironment)
	if !ok {
		return nil, fmt.Errorf("an environment of kind '%s' does not support server operations", env.GetKind())
	}

	return me.CreateServerLifecycleClient(ctx)
}
