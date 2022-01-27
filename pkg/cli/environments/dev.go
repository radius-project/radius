// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/cli/armtemplate/providers"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/k3d"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/localrp"
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

	// Registry is the docker/OCI registry we're using for images.
	Registry *Registry `mapstructure:"registry,omitempty"`

	// APIServerBaseURL is an override for local debugging. This allows us us to run the controller + API Service outside the
	// cluster.
	APIServerBaseURL string     `mapstructure:"apiserverbaseurl,omitempty"`
	Providers        *Providers `mapstructure:"providers"`

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

func (e *LocalEnvironment) GetContainerRegistry() *Registry {
	return e.Registry
}

func (e *LocalEnvironment) HasAzureProvider() bool {
	return e.Providers != nil && e.Providers.AzureProvider != nil
}

func (e *LocalEnvironment) GetAzureProviderDetails() (string, string) {
	if e.HasAzureProvider() {
		return e.Providers.AzureProvider.SubscriptionID, e.Providers.AzureProvider.ResourceGroup
	}

	// Use namespace unless we have an Azure subscription attached.
	return e.Namespace, e.Namespace
}

func (e *LocalEnvironment) CreateDeploymentClient(ctx context.Context) (clients.DeploymentClient, error) {
	baseURL, _, err := kubernetes.CreateAPIServerConnection(e.Context, e.APIServerBaseURL)
	if err != nil {
		return nil, err
	}

	roundTripper, err := kubernetes.CreateRestRoundTripper(e.Context, e.APIServerBaseURL)
	if err != nil {
		return nil, err
	}

	dynamic, err := kubernetes.CreateDynamicClient(e.Context)
	if err != nil {
		return nil, err
	}

	restMapper, err := kubernetes.CreateRESTMapper(e.Context)
	if err != nil {
		return nil, err
	}

	subscriptionID, resourceGroup := e.GetAzureProviderDetails()
	client := localrp.LocalRPDeploymentClient{
		SubscriptionID: subscriptionID,
		ResourceGroup:  resourceGroup,

		// Local Dev supports Radius, Kubernetes, Modules, and optionally Azure
		Providers: map[string]providers.Provider{
			providers.RadiusProviderImport: &providers.AzureProvider{
				Authorizer:     nil, // Anonymous access in local dev
				BaseURL:        baseURL,
				SubscriptionID: e.Namespace, // YES: this supposed to be the namespace since we're talking to the API Service.
				ResourceGroup:  e.Namespace,
				RoundTripper:   roundTripper,
			},
			providers.KubernetesProviderImport: providers.NewK8sProvider(logr.Discard(), dynamic, restMapper),
		},
	}

	client.Providers[providers.DeploymentProviderImport] = &providers.DeploymentProvider{
		DeployFunc: client.DeployNested,
	}

	if e.HasAzureProvider() {
		auth, err := armauth.GetArmAuthorizer()
		if err != nil {
			return nil, err
		}

		client.Providers[providers.AzureProviderImport] = &providers.AzureProvider{
			Authorizer:     auth,
			BaseURL:        baseURL,
			SubscriptionID: subscriptionID,
			ResourceGroup:  resourceGroup,
		}
	}

	return &client, nil
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
	_, connection, err := kubernetes.CreateAPIServerConnection(e.Context, e.APIServerBaseURL)
	if err != nil {
		return nil, err
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
