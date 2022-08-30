// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package connections

import (
	"context"

	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/workspaces"
)

var _ Factory = (*MockFactory)(nil)

type MockFactory struct {
	ApplicationsManagementClient clients.ApplicationsManagementClient
	// TODO support other client types when needed.
}

func (f *MockFactory) CreateDeploymentClient(ctx context.Context, workspace workspaces.Workspace) (clients.DeploymentClient, error) {
	return nil, nil
}

func (f *MockFactory) CreateDiagnosticsClient(ctx context.Context, workspace workspaces.Workspace) (clients.DiagnosticsClient, error) {
	return nil, nil
}

func (f *MockFactory) CreateApplicationsManagementClient(ctx context.Context, workspace workspaces.Workspace) (clients.ApplicationsManagementClient, error) {
	return f.ApplicationsManagementClient, nil
}

func (f *MockFactory) CreateServerLifecycleClient(ctx context.Context, workspace workspaces.Workspace) (clients.ServerLifecycleClient, error) {
	return nil, nil
}
