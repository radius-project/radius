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
	"github.com/project-radius/radius/pkg/cli/k3d"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
)

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
	APIServerBaseURL           string     `mapstructure:"apiserverbaseurl,omitempty"`
	APIDeploymentEngineBaseURL string     `mapstructure:"apideploymentenginebaseurl,omitempty"`
	Providers                  *Providers `mapstructure:"providers"`

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

var _ autorest.Sender = (*devsender)(nil)

type devsender struct {
	RoundTripper http.RoundTripper
}

func (s *devsender) Do(request *http.Request) (*http.Response, error) {
	return s.RoundTripper.RoundTrip(request)
}

func (e *LocalEnvironment) CreateDeploymentClient(ctx context.Context) (clients.DeploymentClient, error) {
	//third parameter indicates this is not UCP env.
	url, roundTripper, err := kubernetes.GetBaseUrlAndRoundTripperForDeploymentEngine(e.APIDeploymentEngineBaseURL, e.Context, false)

	if err != nil {
		return nil, err
	}

	var auth autorest.Authorizer = nil

	subscriptionId, resourceGroup := e.GetAzureProviderDetails()

	tags := map[string]*string{}

	// To support Azure provider today, we need to inform the deployment engine about the Azure subscription.
	// Using tags for now, would love to find a better way to do this if possible.
	if e.HasAzureProvider() {
		tags["azureSubscriptionID"] = &subscriptionId
		tags["azureResourceGroup"] = &resourceGroup

		// Get the location of the resource group for the deployment engine.
		auth, err = armauth.GetArmAuthorizer()
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

	dc := azclients.NewDeploymentsClientWithBaseURI(url, subscriptionId)

	// Poll faster than the default, many deployments are quick
	dc.PollingDelay = 5 * time.Second
	dc.Authorizer = auth

	dc.Sender = &devsender{RoundTripper: roundTripper}

	op := azclients.NewOperationsClientWithBaseUri(url, subscriptionId)
	op.PollingDelay = 5 * time.Second
	op.Sender = &devsender{RoundTripper: roundTripper}
	op.Authorizer = auth

	client := &azure.ARMDeploymentClient{
		Client:           dc,
		OperationsClient: op,
		SubscriptionID:   subscriptionId,
		ResourceGroup:    resourceGroup,
		Tags:             tags,
	}

	return client, nil
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

	_, con, err := kubernetes.CreateAPIServerConnection(e.Context, e.APIServerBaseURL)
	if err != nil {
		return nil, err
	}

	subscriptionID, resourceGroup := e.GetAzureProviderDetails()
	return &azure.ARMDiagnosticsClient{
		K8sTypedClient:   k8sClient,
		RestConfig:       config,
		K8sRuntimeClient: client,
		ResourceClient:   *radclient.NewRadiusResourceClient(con, subscriptionID),
		ResourceGroup:    resourceGroup,
		SubscriptionID:   subscriptionID,
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
