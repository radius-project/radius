// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/cli/azure"
	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/Azure/radius/pkg/cli/k3d"
	"github.com/Azure/radius/pkg/cli/kubernetes"
	"github.com/Azure/radius/pkg/cli/localrp"
)

var _ ServerLifecycleEnvironment = (*LocalEnvironment)(nil)

// LocalEnvironment represents a local test setup for Azure Cloud Radius environment.
type LocalEnvironment struct {
	Name               string `mapstructure:"name" validate:"required"`
	Kind               string `mapstructure:"kind" validate:"required"`
	DefaultApplication string `mapstructure:"defaultapplication,omitempty"`

	Context     string `mapstructure:"context" validate:"required"`
	Namespace   string `mapstructure:"namespace" validate:"required"`
	ClusterName string `mapstructure:"clustername" validate:"required"`
	Registry    string `mapstructure:"registry,omitempty"`

	// URL is an override for local debugging. This allows us us to run the controller + API Service outside the
	// cluster.
	URL       string     `mapstructure:"url,omitempty"`
	Providers *Providers `mapstructure:"providers"`

	// We tolerate and allow extra fields - this helps with forwards compat.
	Properties map[string]interface{} `mapstructure:",remain"`
}

func (e *LocalEnvironment) GetName() string {
	return e.Name
}

func (e *LocalEnvironment) GetKind() string {
	return e.Kind
}

func (e *LocalEnvironment) GetDefaultApplication() string {
	return e.DefaultApplication
}

func (e *LocalEnvironment) GetStatusLink() string {
	return ""
}

func (e *LocalEnvironment) HasAzureProvider() bool {
	return e.Providers != nil && e.Providers.AzureProvider != nil
}

func (e *LocalEnvironment) GetAzureProviderDetails() (string, string) {
	if e.HasAzureProvider() {
		return e.Providers.AzureProvider.SubscriptionID, e.Providers.AzureProvider.ResourceGroup
	}

	return "test-subscription", "test-resource-group"
}

func (e *LocalEnvironment) CreateAPIServiceConnection() (string, *arm.Connection, error) {
	restConfig, err := kubernetes.CreateRestConfig(e.Context)
	if err != nil {
		return "", nil, err
	}

	roundTripper, err := kubernetes.CreateRestRoundTripper(e.Context)
	if err != nil {
		return "", nil, err
	}
	if e.URL != "" {
		// We're not using TLS here, so just use the default roundTripper
		roundTripper = http.DefaultTransport
	}

	azcred := &radclient.AnonymousCredential{}

	baseURL := restConfig.Host + restConfig.APIPath
	if e.URL != "" {
		baseURL = e.URL
	}

	baseURL = fmt.Sprintf("%s/apis/api.radius.dev/v1alpha3", baseURL)
	connection := arm.NewConnection(
		baseURL,
		azcred,
		&arm.ConnectionOptions{
			HTTPClient: &KubernetesRoundTripper{Client: roundTripper},
		})

	return baseURL, connection, nil
}

func (e *LocalEnvironment) CreateDeploymentClient(ctx context.Context) (clients.DeploymentClient, error) {
	// TODO: this will not support Azure resources just yet. Will need to bring in support from radius-local-dev
	// to support multiple providers in the deployment engine.
	baseURL, connection, err := e.CreateAPIServiceConnection()
	if err != nil {
		return nil, fmt.Errorf("failed to create connection: %w", err)
	}

	subscriptionID, resourceGroup := e.GetAzureProviderDetails()
	return &localrp.LocalRPDeploymentClient{
		Authorizer:     nil,
		BaseURL:        baseURL,
		Connection:     connection,
		SubscriptionID: subscriptionID,
		ResourceGroup:  resourceGroup,
	}, nil
}

func (e *LocalEnvironment) CreateDiagnosticsClient(ctx context.Context) (clients.DiagnosticsClient, error) {
	k8sClient, config, err := kubernetes.CreateTypedClient(e.Context)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.CreateRuntimeClient(e.Context, kubernetes.Scheme)
	if err != nil {
		return nil, err
	}

	return &kubernetes.KubernetesDiagnosticsClient{
		K8sClient:  k8sClient,
		Client:     client,
		RestConfig: config,
		Namespace:  e.Namespace,
	}, nil
}

func (e *LocalEnvironment) CreateManagementClient(ctx context.Context) (clients.ManagementClient, error) {
	_, connection, err := e.CreateAPIServiceConnection()
	if err != nil {
		return nil, fmt.Errorf("failed to create connection: %w", err)
	}

	subscriptionID, resourceGroup := e.GetAzureProviderDetails()

	return &azure.ARMManagementClient{
		Connection:      connection,
		SubscriptionID:  subscriptionID,
		ResourceGroup:   resourceGroup,
		EnvironmentName: e.Name,
	}, nil
}

func (e *LocalEnvironment) CreateServerLifecycleClient(ctx context.Context) (clients.ServerLifecycleClient, error) {
	return &k3d.ServerLifecycleClient{
		ClusterName: e.ClusterName,
	}, nil
}
