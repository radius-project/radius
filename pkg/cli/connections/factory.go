// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package connections

import (
	"context"
	"errors"
	"fmt"
	"strings"

	aztoken "github.com/project-radius/radius/pkg/azure/tokencredentials"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/clients_new/generated"
	"github.com/project-radius/radius/pkg/cli/deployment"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/ucp"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/sdk"
	sdkclients "github.com/project-radius/radius/pkg/sdk/clients"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

// DefaultFactory provides easy access to the default implementation of the factory. DO NOT modify this in your code. Even if it's for tests. DO NOT DO IT.
var DefaultFactory = &impl{}

// ConnectionFactory is a mockable abstraction for our client-server interations.
type Factory interface {
	CreateDeploymentClient(ctx context.Context, workspace workspaces.Workspace) (clients.DeploymentClient, error)
	CreateDiagnosticsClient(ctx context.Context, workspace workspaces.Workspace) (clients.DiagnosticsClient, error)
	CreateApplicationsManagementClient(ctx context.Context, workspace workspaces.Workspace) (clients.ApplicationsManagementClient, error)
	CreateCloudProviderManagementClient(ctx context.Context, workspace workspaces.Workspace) (clients.CloudProviderManagementClient, error)
}

var _ Factory = (*impl)(nil)

type impl struct {
}

func (i *impl) CreateDeploymentClient(ctx context.Context, workspace workspaces.Workspace) (clients.DeploymentClient, error) {
	connection, err := workspace.Connect()
	if err != nil {
		return nil, err
	}

	err = sdk.TestConnection(ctx, connection)
	if errors.Is(err, &sdk.ErrRadiusNotInstalled{}) {
		return nil, &cli.FriendlyError{Message: err.Error()}
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
		RadiusResourceGroup: id.FindScope(resources.ResourceGroupsSegment),
		AzProvider:          workspace.ProviderConfig.Azure,
		AWSProvider:         workspace.ProviderConfig.AWS,
	}, nil
}

func (i *impl) CreateDiagnosticsClient(ctx context.Context, workspace workspaces.Workspace) (clients.DiagnosticsClient, error) {
	connection, err := workspace.Connect()
	if err != nil {
		return nil, err
	}

	err = sdk.TestConnection(ctx, connection)
	if errors.Is(err, &sdk.ErrRadiusNotInstalled{}) {
		return nil, &cli.FriendlyError{Message: err.Error()}
	} else if err != nil {
		return nil, err
	}

	connectionConfig, err := workspace.ConnectionConfig()
	if err != nil {
		return nil, err
	}

	switch c := connectionConfig.(type) {
	case *workspaces.KubernetesConnectionConfig:
		k8sClient, config, err := kubernetes.CreateTypedClient(c.Context)
		if err != nil {
			return nil, err
		}
		client, err := kubernetes.CreateRuntimeClient(c.Context, kubernetes.Scheme)
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

func (*impl) CreateApplicationsManagementClient(ctx context.Context, workspace workspaces.Workspace) (clients.ApplicationsManagementClient, error) {
	connection, err := workspace.Connect()
	if err != nil {
		return nil, err
	}

	err = sdk.TestConnection(ctx, connection)
	if errors.Is(err, &sdk.ErrRadiusNotInstalled{}) {
		return nil, &cli.FriendlyError{Message: err.Error()}
	} else if err != nil {
		return nil, err
	}

	return &ucp.ARMApplicationsManagementClient{
		// The client expects root scope without a leading /
		RootScope:     strings.TrimPrefix(workspace.Scope, resources.SegmentSeparator),
		ClientOptions: sdk.NewClientOptions(connection),
	}, nil
}

//nolint:all
func (*impl) CreateCloudProviderManagementClient(ctx context.Context, workspace workspaces.Workspace) (clients.CloudProviderManagementClient, error) {
	return nil, errors.New("this feature is currently not supported")
}
