// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"fmt"

	"github.com/Azure/radius/pkg/rad/clients"
)

const (
	KindAzureCloud                   = "azure"
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
	CreateDeploymentClient() (clients.DeploymentClient, error)
}

func CreateDeploymentClient(env Environment) (clients.DeploymentClient, error) {
	de, ok := env.(DeploymentEnvironment)
	if !ok {
		return nil, fmt.Errorf("an environment of kind '%s' does not support deployment", env.GetKind())
	}

	return de.CreateDeploymentClient()
}

type DiagnosticsEnvironment interface {
	CreateDiagnosticsClient() (clients.DiagnosticsClient, error)
}

func CreateDiagnosticsClient(env Environment) (clients.DiagnosticsClient, error) {
	de, ok := env.(DiagnosticsEnvironment)
	if !ok {
		return nil, fmt.Errorf("an environment of kind '%s' does not support diagnostics operations", env.GetKind())
	}

	return de.CreateDiagnosticsClient()
}

type ManagementEnvironment interface {
	CreateManagementClient() (clients.ManagementClient, error)
}

func CreateManagementClient(env Environment) (clients.ManagementClient, error) {
	me, ok := env.(ManagementEnvironment)
	if !ok {
		return nil, fmt.Errorf("an environment of kind '%s' does not support management operations", env.GetKind())
	}

	return me.CreateManagementClient()
}
