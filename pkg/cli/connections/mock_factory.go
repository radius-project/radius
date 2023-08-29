/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package connections

import (
	"context"

	"github.com/radius-project/radius/pkg/cli/clients"
	cli_credential "github.com/radius-project/radius/pkg/cli/credential"
	"github.com/radius-project/radius/pkg/cli/workspaces"
)

var _ Factory = (*MockFactory)(nil)

type MockFactory struct {
	ApplicationsManagementClient clients.ApplicationsManagementClient
	CredentialManagementClient   cli_credential.CredentialManagementClient
	DiagnosticsClient            clients.DiagnosticsClient
}

// CreateDeploymentClient function takes in a context and a workspace and returns a DeploymentClient and an error, if any.
func (f *MockFactory) CreateDeploymentClient(ctx context.Context, workspace workspaces.Workspace) (clients.DeploymentClient, error) {
	return nil, nil
}

// CreateDiagnosticsClient function takes in a context and a workspace and returns a DiagnosticsClient without any errors.
func (f *MockFactory) CreateDiagnosticsClient(ctx context.Context, workspace workspaces.Workspace) (clients.DiagnosticsClient, error) {
	return f.DiagnosticsClient, nil
}

// CreateApplicationsManagementClient function takes in a context and a workspace and returns an ApplicationsManagementClient
// and an error if one occurs.
func (f *MockFactory) CreateApplicationsManagementClient(ctx context.Context, workspace workspaces.Workspace) (clients.ApplicationsManagementClient, error) {
	return f.ApplicationsManagementClient, nil
}

// CreateCredentialManagementClient function takes in a context and a workspace and returns a CredentialManagementClient and does not return an error.
func (f *MockFactory) CreateCredentialManagementClient(ctx context.Context, workspace workspaces.Workspace) (cli_credential.CredentialManagementClient, error) {
	return f.CredentialManagementClient, nil
}
