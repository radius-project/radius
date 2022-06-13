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

	"github.com/project-radius/radius/pkg/azure/armauth"
	azclients "github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
)

// KubernetesEnvironment represents a Kubernetes Radius environment.
type KubernetesEnvironment struct {
	Name                     string     `mapstructure:"name" validate:"required"`
	Kind                     string     `mapstructure:"kind" validate:"required"`
	Context                  string     `mapstructure:"context" validate:"required"`
	Namespace                string     `mapstructure:"namespace" validate:"required"`
	DefaultApplication       string     `mapstructure:"defaultapplication,omitempty"`
	Providers                *Providers `mapstructure:"providers"`
	RadiusRPLocalURL         string     `mapstructure:"radiusrplocalurl,omitempty"`
	DeploymentEngineLocalURL string     `mapstructure:"deploymentenginelocalurl,omitempty"`
	UCPLocalURL              string     `mapstructure:"ucplocalurl,omitempty"`
	EnableUCP                bool       `mapstructure:"enableucp,omitempty"`

	// We tolerate and allow extra fields - this helps with forwards compat.
	Properties map[string]interface{} `mapstructure:",remain"`
}

func (e *KubernetesEnvironment) GetName() string {
	return e.Name
}

func (e *KubernetesEnvironment) GetKind() string {
	return e.Kind
}

func (e *KubernetesEnvironment) GetEnableUCP() bool {
	return e.EnableUCP
}

func (e *KubernetesEnvironment) GetDefaultApplication() string {
	return e.DefaultApplication
}

func (e *KubernetesEnvironment) GetContainerRegistry() *Registry {
	return nil
}

// No Status Link for kubernetes
func (e *KubernetesEnvironment) GetStatusLink() string {
	return ""
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
	url, roundTripper, err := kubernetes.GetBaseUrlAndRoundTripperForDeploymentEngine(e.DeploymentEngineLocalURL, e.UCPLocalURL, e.Context, e.EnableUCP)

	if err != nil {
		return nil, err
	}

	subscriptionId, resourceGroup := e.GetAzureProviderDetails()
	tags := map[string]*string{}
	// To support Azure provider today, we need to inform the deployment engine about the Azure subscription.
	// Using tags for now, would love to find a better way to do this if possible.
	if e.HasAzureProvider() {
		tags["azureSubscriptionID"] = &subscriptionId
		tags["azureResourceGroup"] = &resourceGroup

		// Get the location of the resource group for the deployment engine.
		auth, err := armauth.GetArmAuthorizer()
		if err != nil {
			return nil, err
		}

		rgClient := azclients.NewGroupsClient(subscriptionId, auth)
		resp, err := rgClient.Get(ctx, resourceGroup)
		if err != nil {
			return nil, err
		}
		tags["azureLocation"] = resp.Location
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
		SubscriptionID:   e.Namespace,
		ResourceGroup:    e.Namespace,
		Tags:             tags,
		EnableUCP:        e.EnableUCP,
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
		ResourceGroup:    e.Namespace,
		SubscriptionID:   e.Namespace,
	}, nil
}

func (e *KubernetesEnvironment) CreateManagementClient(ctx context.Context) (clients.ManagementClient, error) {
	_, connection, err := kubernetes.CreateAPIServerConnection(e.Context, e.RadiusRPLocalURL)
	if err != nil {
		return nil, err
	}

	return &azure.ARMManagementClient{
		EnvironmentName: e.Name,
		Connection:      connection,
		ResourceGroup:   e.Namespace, // Temporarily set resource group and subscription id to the namespace
		SubscriptionID:  e.Namespace,
	}, nil
}
