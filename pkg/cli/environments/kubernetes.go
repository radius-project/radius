// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"net/http"
	"time"

	"github.com/Azure/go-autorest/autorest"

	azclients "github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/ucp"
)

// KubernetesEnvironment represents a Kubernetes Radius environment.
type KubernetesEnvironment struct {
	RadiusEnvironment `mapstructure:",squash"`
}

func (e *KubernetesEnvironment) GetName() string {
	return e.Name
}

func (e *KubernetesEnvironment) GetKind() string {
	return e.Kind
}

func (e *KubernetesEnvironment) GetDefaultApplication() string {
	return e.DefaultApplication
}

func (e *KubernetesEnvironment) GetKubeContext() string {
	return e.Context
}

func (e *KubernetesEnvironment) GetContainerRegistry() *Registry {
	return nil
}

// No Status Link for kubernetes
func (e *KubernetesEnvironment) GetStatusLink() string {
	return ""
}

func (e *KubernetesEnvironment) GetProviders() *Providers {
	return e.Providers
}

var _ autorest.Sender = (*sender)(nil)

type sender struct {
	RoundTripper http.RoundTripper
}

func (s *sender) Do(request *http.Request) (*http.Response, error) {
	return s.RoundTripper.RoundTrip(request)
}

func (e *KubernetesEnvironment) HasAzureProvider() bool {
	return e.Providers != nil && e.Providers.AzureProvider != nil
}

func (e *KubernetesEnvironment) GetAzureProviderDetails() (string, string) {
	if e.HasAzureProvider() {
		return e.Providers.AzureProvider.SubscriptionID, e.Providers.AzureProvider.ResourceGroup
	}

	// Use namespace unless we have an Azure subscription attached.
	return e.Namespace, e.Namespace
}

func (e *KubernetesEnvironment) CreateDeploymentClient(ctx context.Context) (clients.DeploymentClient, error) {
	url, roundTripper, err := kubernetes.GetBaseUrlAndRoundTripperForDeploymentEngine(e.DeploymentEngineLocalURL, e.UCPLocalURL, e.Context)

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
	return &azure.ResouceDeploymentClient{
		Client:           dc,
		OperationsClient: op,
		ResourceGroup:    e.UCPResourceGroupName,
	}, nil
}

func (e *KubernetesEnvironment) CreateLegacyDiagnosticsClient(ctx context.Context) (clients.DiagnosticsClient, error) {
	k8sClient, config, err := kubernetes.CreateTypedClient(e.Context)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.CreateRuntimeClient(e.Context, kubernetes.Scheme)
	if err != nil {
		return nil, err
	}

	_, con, err := kubernetes.CreateAPIServerConnection(e.Context, e.RadiusRPLocalURL)
	if err != nil {
		return nil, err
	}

	return &azure.ARMDiagnosticsClient{
		K8sTypedClient:   k8sClient,
		RestConfig:       config,
		K8sRuntimeClient: client,
		ResourceClient:   *radclient.NewRadiusResourceClient(con, e.Namespace),
		ResourceGroup:    e.Namespace,
		SubscriptionID:   e.Namespace,
	}, nil
}

func (e *KubernetesEnvironment) CreateDiagnosticsClient(ctx context.Context) (clients.DiagnosticsClient, error) {
	k8sClient, config, err := kubernetes.CreateTypedClient(e.Context)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.CreateRuntimeClient(e.Context, kubernetes.Scheme)
	if err != nil {
		return nil, err
	}

	_, con, err := kubernetes.CreateAPIServerConnection(e.Context, e.RadiusRPLocalURL)
	if err != nil {
		return nil, err
	}

	return &azure.ARMDiagnosticsClient{
		K8sTypedClient:   k8sClient,
		RestConfig:       config,
		K8sRuntimeClient: client,
		ResourceClient:   *radclient.NewRadiusResourceClient(con, e.Namespace),
		ResourceGroup:    e.UCPResourceGroupName,
	}, nil
}

func (e *KubernetesEnvironment) CreateLegacyManagementClient(ctx context.Context) (clients.LegacyManagementClient, error) {
	_, connection, err := kubernetes.CreateAPIServerConnection(e.Context, e.RadiusRPLocalURL)
	if err != nil {
		return nil, err
	}

	return &azure.LegacyARMManagementClient{
		EnvironmentName: e.Name,
		Connection:      connection,
		ResourceGroup:   e.Namespace, // Temporarily set resource group and subscription id to the namespace
		SubscriptionID:  e.Namespace,
	}, nil
}

func (e *KubernetesEnvironment) CreateApplicationsManagementClient(ctx context.Context) (clients.ApplicationsManagementClient, error) {
	_, connection, err := kubernetes.CreateAPIServerConnection(e.Context, e.UCPLocalURL)
	if err != nil {
		return nil, err
	}

	//TODO: add support to rad env init to write resourcegroup to config.yaml as scope
	// and use that as RootScope value
	return &ucp.ARMApplicationsManagementClient{
		EnvironmentName: e.Name,
		Connection:      connection,
		RootScope:       "planes/radius/local/ResourceGroups/" + e.UCPResourceGroupName,
	}, nil
}
