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

	"github.com/project-radius/radius/pkg/cli/clients"
	cli_credential "github.com/project-radius/radius/pkg/cli/credential"
	"github.com/project-radius/radius/pkg/cli/workspaces"
)

var _ Factory = (*MockFactory)(nil)

type MockFactory struct {
	ApplicationsManagementClient clients.ApplicationsManagementClient
	CredentialManagementClient   cli_credential.CredentialManagementClient
	DiagnosticsClient            clients.DiagnosticsClient
	// TODO support other client types when needed.
}

// # Function Explanation
// 
//	MockFactory.CreateDeploymentClient creates a DeploymentClient and returns it, or returns an error if one occurs.
func (f *MockFactory) CreateDeploymentClient(ctx context.Context, workspace workspaces.Workspace) (clients.DeploymentClient, error) {
	return nil, nil
}

// # Function Explanation
// 
//	MockFactory's CreateDiagnosticsClient function creates a DiagnosticsClient and always returns nil as an error, allowing 
//	callers to use the DiagnosticsClient without worrying about any errors.
func (f *MockFactory) CreateDiagnosticsClient(ctx context.Context, workspace workspaces.Workspace) (clients.DiagnosticsClient, error) {
	return f.DiagnosticsClient, nil
}

// # Function Explanation
// 
//	MockFactory.CreateApplicationsManagementClient creates a mock ApplicationsManagementClient and returns it, or an error 
//	if one occurs.
func (f *MockFactory) CreateApplicationsManagementClient(ctx context.Context, workspace workspaces.Workspace) (clients.ApplicationsManagementClient, error) {
	return f.ApplicationsManagementClient, nil
}

// # Function Explanation
// 
//	MockFactory.CreateCredentialManagementClient creates a CredentialManagementClient and returns it, or an error if one 
//	occurs.
func (f *MockFactory) CreateCredentialManagementClient(ctx context.Context, workspace workspaces.Workspace) (cli_credential.CredentialManagementClient, error) {
	return f.CredentialManagementClient, nil
}
