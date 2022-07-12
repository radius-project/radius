// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/clients"
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
	GetKubeContext() string
	GetId() string

	// GetStatusLink provides an optional URL for display of the environment.
	GetStatusLink() string

	// GetContainerRegistry provides an optional container registry override. The registry is used
	// by the 'rad app ...' family of commands for development purposes.
	GetContainerRegistry() *Registry

	GetProviders() *Providers
}

// Registry represent the configuration for a container registry.
type Registry struct {
	// PushEndpoint is the endpoint used for push commands. For a local container registry this hostname
	// is expected to be accessible from the host machine.
	PushEndpoint string `mapstructure:"pushendpoint" validate:"required"`

	// PullEndpoint is the endpoing used to pull by the runtime. For a local container registry this hostname
	// is expected to be accessible by the runtime. Can be the same as PushEndpoint if the registry has a routable
	// address.
	PullEndpoint string `mapstructure:"pullendpoint" validate:"required"`
}

type Providers struct {
	AzureProvider *azure.Provider `mapstructure:"azure,omitempty"`
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

type LegacyManagementEnvironment interface {
}

type ApplicationsManagementEnvironment interface {
	CreateApplicationsManagementClient(ctx context.Context) (clients.ApplicationsManagementClient, error)
}

func CreateApplicationsManagementClient(ctx context.Context, env Environment) (clients.ApplicationsManagementClient, error) {
	me, ok := env.(ApplicationsManagementEnvironment)
	if !ok {
		return nil, fmt.Errorf("an environment of kind '%s' does not support management operations", env.GetKind())
	}

	return me.CreateApplicationsManagementClient(ctx)
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
