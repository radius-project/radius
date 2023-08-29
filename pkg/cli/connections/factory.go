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
	"errors"
	"fmt"
	"strings"

	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	cli_credential "github.com/radius-project/radius/pkg/cli/credential"
	"github.com/radius-project/radius/pkg/cli/deployment"
	"github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/sdk"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_radius "github.com/radius-project/radius/pkg/ucp/resources/radius"
)

// DefaultFactory provides easy access to the default implementation of the factory. DO NOT modify this in your code. Even if it's for tests. DO NOT DO IT.
var DefaultFactory = &impl{}

// ConnectionFactory is a mockable abstraction for our client-server interactions.
type Factory interface {
	CreateDeploymentClient(ctx context.Context, workspace workspaces.Workspace) (clients.DeploymentClient, error)
	CreateDiagnosticsClient(ctx context.Context, workspace workspaces.Workspace) (clients.DiagnosticsClient, error)
	CreateApplicationsManagementClient(ctx context.Context, workspace workspaces.Workspace) (clients.ApplicationsManagementClient, error)
	CreateCredentialManagementClient(ctx context.Context, workspace workspaces.Workspace) (cli_credential.CredentialManagementClient, error)
}

var _ Factory = (*impl)(nil)

type impl struct {
}

// CreateDeploymentClient connects to a workspace, tests the connection, creates a deployment client and an operations
// client, and returns them along with the resource group name. It returns an error if any of the steps fail.
func (i *impl) CreateDeploymentClient(ctx context.Context, workspace workspaces.Workspace) (clients.DeploymentClient, error) {
	connection, err := workspace.Connect()
	if err != nil {
		return nil, err
	}

	err = sdk.TestConnection(ctx, connection)
	if errors.Is(err, &sdk.ErrRadiusNotInstalled{}) {
		return nil, clierrors.MessageWithCause(err, "Could not connect to Radius.")
	} else if err != nil {
		return nil, err
	}

	armClientOptions := sdk.NewClientOptions(connection)
	dc, err := sdkclients.NewResourceDeploymentsClient(&sdkclients.Options{
		Cred:             &aztoken.AnonymousCredential{},
		BaseURI:          connection.Endpoint(),
		ARMClientOptions: armClientOptions,
	})
	if err != nil {
		return nil, err
	}

	doc, err := sdkclients.NewResourceDeploymentOperationsClient(&sdkclients.Options{
		Cred:             &aztoken.AnonymousCredential{},
		BaseURI:          connection.Endpoint(),
		ARMClientOptions: armClientOptions,
	})
	if err != nil {
		return nil, err
	}

	// This client wants a resource group name, but we store the ID instead, so compute that.
	id, err := resources.ParseScope(workspace.Scope)
	if err != nil {
		return nil, err
	}

	return &deployment.ResourceDeploymentClient{
		Client:              dc,
		OperationsClient:    doc,
		RadiusResourceGroup: id.FindScope(resources_radius.ScopeResourceGroups),
	}, nil
}

// CreateDiagnosticsClient creates a DiagnosticsClient by connecting to a workspace, testing the connection, and creating
// clients for applications, containers, environments, and gateways. If an error occurs, it is returned.
func (i *impl) CreateDiagnosticsClient(ctx context.Context, workspace workspaces.Workspace) (clients.DiagnosticsClient, error) {
	connection, err := workspace.Connect()
	if err != nil {
		return nil, err
	}

	err = sdk.TestConnection(ctx, connection)
	if errors.Is(err, &sdk.ErrRadiusNotInstalled{}) {
		return nil, clierrors.MessageWithCause(err, "Could not connect to Radius.")
	} else if err != nil {
		return nil, err
	}

	connectionConfig, err := workspace.ConnectionConfig()
	if err != nil {
		return nil, err
	}

	switch c := connectionConfig.(type) {
	case *workspaces.KubernetesConnectionConfig:
		k8sClient, config, err := kubernetes.NewClientset(c.Context)
		if err != nil {
			return nil, err
		}
		client, err := kubernetes.NewRuntimeClient(c.Context, kubernetes.Scheme)
		if err != nil {
			return nil, err
		}

		clientOpts := sdk.NewClientOptions(connection)
		appClient, err := generated.NewGenericResourcesClient(workspace.Scope, "Applications.Core/applications", &aztoken.AnonymousCredential{}, clientOpts)
		if err != nil {
			return nil, err
		}

		cntrClient, err := generated.NewGenericResourcesClient(workspace.Scope, "Applications.Core/containers", &aztoken.AnonymousCredential{}, clientOpts)
		if err != nil {
			return nil, err
		}

		envClient, err := generated.NewGenericResourcesClient(workspace.Scope, "Applications.Core/environments", &aztoken.AnonymousCredential{}, clientOpts)
		if err != nil {
			return nil, err
		}

		gwClient, err := generated.NewGenericResourcesClient(workspace.Scope, "Applications.Core/gateways", &aztoken.AnonymousCredential{}, clientOpts)
		if err != nil {
			return nil, err
		}

		return &deployment.ARMDiagnosticsClient{
			K8sTypedClient:    k8sClient,
			RestConfig:        config,
			K8sRuntimeClient:  client,
			ApplicationClient: *appClient,
			ContainerClient:   *cntrClient,
			EnvironmentClient: *envClient,
			GatewayClient:     *gwClient,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported connection type: %+v", connection)
	}
}

// CreateApplicationsManagementClient connects to the workspace, tests the connection, and returns a
// UCPApplicationsManagementClient if successful, or an error if unsuccessful.
func (*impl) CreateApplicationsManagementClient(ctx context.Context, workspace workspaces.Workspace) (clients.ApplicationsManagementClient, error) {
	connection, err := workspace.Connect()
	if err != nil {
		return nil, err
	}

	err = sdk.TestConnection(ctx, connection)
	if errors.Is(err, &sdk.ErrRadiusNotInstalled{}) {
		return nil, clierrors.MessageWithCause(err, "Could not connect to Radius.")
	} else if err != nil {
		return nil, err
	}

	return &clients.UCPApplicationsManagementClient{
		// The client expects root scope without a leading /
		RootScope:     strings.TrimPrefix(workspace.Scope, resources.SegmentSeparator),
		ClientOptions: sdk.NewClientOptions(connection),
	}, nil
}

// Creates Credential management client to interact with server side credentials.
//

// CreateCredentialManagementClient establishes a connection to a workspace, tests the connection, creates Azure and AWS
// credential clients, and returns a UCPCredentialManagementClient. An error is returned if any of the steps fail.
func (*impl) CreateCredentialManagementClient(ctx context.Context, workspace workspaces.Workspace) (cli_credential.CredentialManagementClient, error) {
	connection, err := workspace.Connect()
	if err != nil {
		return nil, err
	}

	err = sdk.TestConnection(ctx, connection)
	if errors.Is(err, &sdk.ErrRadiusNotInstalled{}) {
		return nil, clierrors.MessageWithCause(err, "Could not connect to Radius.")
	} else if err != nil {
		return nil, err
	}

	clientOptions := sdk.NewClientOptions(connection)

	azureCredentialClient, err := v20220901privatepreview.NewAzureCredentialClient(&aztoken.AnonymousCredential{}, clientOptions)
	if err != nil {
		return nil, err
	}

	awsCredentialClient, err := v20220901privatepreview.NewAwsCredentialClient(&aztoken.AnonymousCredential{}, clientOptions)
	if err != nil {
		return nil, err
	}

	azureCMClient := &cli_credential.AzureCredentialManagementClient{
		AzureCredentialClient: *azureCredentialClient,
	}

	awsCMClient := &cli_credential.AWSCredentialManagementClient{
		AWSCredentialClient: *awsCredentialClient,
	}

	cpClient := &cli_credential.UCPCredentialManagementClient{
		AzClient:  azureCMClient,
		AWSClient: awsCMClient,
	}

	return cpClient, nil
}
