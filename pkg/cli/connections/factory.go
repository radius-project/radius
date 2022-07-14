// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package connections

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/go-autorest/autorest"
	azclients "github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/clients_new/generated"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/ucp"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

// DefaultFactory provides easy access to the default implementation of the factory. DO NOT modify this in your code. Even if it's for tests. DO NOT DO IT.
var DefaultFactory = &impl{}

// ConnectionFactory is a mockable abstraction for our client-server interations.
type Factory interface {
	CreateDeploymentClient(ctx context.Context, workspace workspaces.Workspace) (clients.DeploymentClient, error)
	CreateDiagnosticsClient(ctx context.Context, workspace workspaces.Workspace) (clients.DiagnosticsClient, error)
	CreateApplicationsManagementClient(ctx context.Context, workspace workspaces.Workspace) (clients.ApplicationsManagementClient, error)
	CreateServerLifecycleClient(ctx context.Context, workspace workspaces.Workspace) (clients.ServerLifecycleClient, error)
}

var _ Factory = (*impl)(nil)

type impl struct {
}

func (*impl) CreateDeploymentClient(ctx context.Context, workspace workspaces.Workspace) (clients.DeploymentClient, error) {
	connection, err := workspace.Connect()
	if err != nil {
		return nil, err
	}

	switch c := connection.(type) {
	case *workspaces.KubernetesConnection:
		url, roundTripper, err := kubernetes.GetBaseUrlAndRoundTripperForDeploymentEngine(c.Overrides.UCP, c.Context)

		if err != nil {
			return nil, err
		}

		dc := azclients.NewResourceDeploymentClientWithBaseURI(url)

		// Poll faster than the default, many deployments are quick
		dc.PollingDelay = 5 * time.Second

		dc.Sender = &sender{RoundTripper: roundTripper}

		op := azclients.NewResourceDeploymentOperationsClientWithBaseURI(url)
		op.PollingDelay = 5 * time.Second
		op.Sender = &sender{RoundTripper: roundTripper}

		// This client wants a resource group name, but we store the ID instead, so compute that.
		id, err := resources.Parse(workspace.Scope)
		if err != nil {
			return nil, err
		}

		return &azure.ResouceDeploymentClient{
			Client:              dc,
			OperationsClient:    op,
			RadiusResourceGroup: id.FindScope(resources.ResourceGroupsSegment),
			AzProvider:          workspace.ProviderConfig.Azure,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported connection type: %+v", connection)
	}
}

func (*impl) CreateDiagnosticsClient(ctx context.Context, workspace workspaces.Workspace) (clients.DiagnosticsClient, error) {
	connection, err := workspace.Connect()
	if err != nil {
		return nil, err
	}

	switch c := connection.(type) {
	case *workspaces.KubernetesConnection:
		k8sClient, config, err := kubernetes.CreateTypedClient(c.Context)
		if err != nil {
			return nil, err
		}
		client, err := kubernetes.CreateRuntimeClient(c.Context, kubernetes.Scheme)
		if err != nil {
			return nil, err
		}

		_, con, err := kubernetes.CreateAPIServerConnection(c.Context, c.Overrides.UCP)
		if err != nil {
			return nil, err
		}

		return &azure.ARMDiagnosticsClient{
			K8sTypedClient:    k8sClient,
			RestConfig:        config,
			K8sRuntimeClient:  client,
			ApplicationClient: *generated.NewGenericResourcesClient(con, workspace.Scope, "Applications.Core/applications"),
			ContainerClient:   *generated.NewGenericResourcesClient(con, workspace.Scope, "Applications.Core/containers"),
			EnvironmentClient:   *generated.NewGenericResourcesClient(con, workspace.Scope, "Applications.Core/environments"),
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

	switch c := connection.(type) {
	case *workspaces.KubernetesConnection:
		_, connection, err := kubernetes.CreateAPIServerConnection(c.Context, c.Overrides.UCP)
		if err != nil {
			return nil, err
		}

		return &ucp.ARMApplicationsManagementClient{
			Connection: connection,

			// The client expects root scope without a leading /
			RootScope: strings.TrimPrefix(workspace.Scope, resources.SegmentSeparator),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported connection type: %+v", connection)
	}
}

func (*impl) CreateServerLifecycleClient(ctx context.Context, workspace workspaces.Workspace) (clients.ServerLifecycleClient, error) {
	return nil, errors.New("this feature is currently not supported")
}

var _ autorest.Sender = (*sender)(nil)

type sender struct {
	RoundTripper http.RoundTripper
}

func (s *sender) Do(request *http.Request) (*http.Response, error) {
	return s.RoundTripper.RoundTrip(request)
}
