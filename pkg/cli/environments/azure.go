// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"fmt"
	"time"

	"github.com/project-radius/radius/pkg/azure/armauth"
	azclients "github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/ucp"
)

func RequireAzureCloud(e Environment) (*AzureCloudEnvironment, error) {
	az, ok := e.(*AzureCloudEnvironment)
	if !ok {
		return nil, fmt.Errorf("an '%v' environment is required but the kind was '%v'", KindAzureCloud, e.GetKind())
	}

	return az, nil
}

// AzureCloudEnvironment represents an Azure Cloud Radius environment.
type AzureCloudEnvironment struct {
	RadiusEnvironment `mapstructure:",squash"`
	ClusterName       string `mapstructure:"clustername" validate:"required"`
	SubscriptionID    string `mapstructure:"subscriptionid" validate:"required"`
	ResourceGroup     string `mapstructure:"resourcegroup" validate:"required"`
}

func (e *AzureCloudEnvironment) GetName() string {
	return e.Name
}

func (e *AzureCloudEnvironment) GetKind() string {
	return e.Kind
}

func (e *AzureCloudEnvironment) GetDefaultApplication() string {
	return e.DefaultApplication
}

func (e *AzureCloudEnvironment) GetKubeContext() string {
	return e.Context
}

func (e *AzureCloudEnvironment) GetContainerRegistry() *Registry {
	return nil
}

func (e *AzureCloudEnvironment) GetId() string {
	return e.Id
}

func (e *AzureCloudEnvironment) GetStatusLink() string {
	// If there's a problem generating the status link, we don't want to fail noisily, just skip the link.
	url, err := azure.GenerateAzureEnvUrl(e.SubscriptionID, e.ResourceGroup)
	if err != nil {
		return ""
	}

	return url
}

func (e *AzureCloudEnvironment) GetProviders() *Providers {
	return nil
}

func (e *AzureCloudEnvironment) CreateDeploymentClient(ctx context.Context) (clients.DeploymentClient, error) {
	url, roundTripper, err := kubernetes.GetBaseUrlAndRoundTripperForDeploymentEngine(
		e.DeploymentEngineLocalURL,
		e.UCPLocalURL,
		e.Context,
	)
	if err != nil {
		return nil, err
	}

	auth, err := armauth.GetArmAuthorizer()
	if err != nil {
		return nil, err
	}

	tags := map[string]*string{}

	tags["azureSubscriptionID"] = &e.SubscriptionID
	tags["azureResourceGroup"] = &e.ResourceGroup

	rgClient := azclients.NewGroupsClient(e.SubscriptionID, auth)
	resp, err := rgClient.Get(ctx, e.ResourceGroup)
	if err != nil {
		return nil, err
	}
	tags["azureLocation"] = resp.Location

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
		SubscriptionID:   e.SubscriptionID,
		ResourceGroup:    e.UCPResourceGroupName,
		Tags:             tags,
	}, nil
}

func (e *AzureCloudEnvironment) CreateLegacyDiagnosticsClient(ctx context.Context) (clients.DiagnosticsClient, error) {
	k8sTypedClient, config, err := kubernetes.CreateTypedClient(e.Context)
	if err != nil {
		return nil, err
	}

	k8sRuntimeclient, err := kubernetes.CreateRuntimeClient(e.Context, kubernetes.Scheme)
	if err != nil {
		return nil, err
	}

	_, con, err := kubernetes.CreateAPIServerConnection(e.Context, e.RadiusRPLocalURL)
	if err != nil {
		return nil, err
	}

	return &azure.ARMDiagnosticsClient{
		K8sTypedClient:   k8sTypedClient,
		RestConfig:       config,
		K8sRuntimeClient: k8sRuntimeclient,
		ResourceClient:   *radclient.NewRadiusResourceClient(con, e.SubscriptionID),
		ResourceGroup:    e.ResourceGroup,
		SubscriptionID:   e.SubscriptionID,
	}, nil
}

func (e *AzureCloudEnvironment) CreateDiagnosticsClient(ctx context.Context) (clients.DiagnosticsClient, error) {
	k8sTypedClient, config, err := kubernetes.CreateTypedClient(e.Context)
	if err != nil {
		return nil, err
	}

	k8sRuntimeclient, err := kubernetes.CreateRuntimeClient(e.Context, kubernetes.Scheme)
	if err != nil {
		return nil, err
	}

	_, con, err := kubernetes.CreateAPIServerConnection(e.Context, e.RadiusRPLocalURL)
	if err != nil {
		return nil, err
	}

	return &azure.ARMDiagnosticsClient{
		K8sTypedClient:   k8sTypedClient,
		RestConfig:       config,
		K8sRuntimeClient: k8sRuntimeclient,
		ResourceClient:   *radclient.NewRadiusResourceClient(con, e.SubscriptionID),
		ResourceGroup:    e.UCPResourceGroupName,
		SubscriptionID:   e.SubscriptionID,
	}, nil
}

func (e *AzureCloudEnvironment) CreateLegacyManagementClient(ctx context.Context) (clients.LegacyManagementClient, error) {
	_, connection, err := kubernetes.CreateAPIServerConnection(e.Context, e.RadiusRPLocalURL)
	if err != nil {
		return nil, err
	}

	return &azure.LegacyARMManagementClient{
		EnvironmentName: e.Name,
		Connection:      connection,
		ResourceGroup:   e.ResourceGroup,
		SubscriptionID:  e.SubscriptionID,
	}, nil
}

func (e *AzureCloudEnvironment) CreateApplicationsManagementClient(ctx context.Context) (clients.ApplicationsManagementClient, error) {
	_, connection, err := kubernetes.CreateAPIServerConnection(e.Context, e.UCPLocalURL)
	if err != nil {
		return nil, err
	}

	return &ucp.ARMApplicationsManagementClient{
		EnvironmentName: e.Name,
		Connection:      connection,
		RootScope:       "/Subscriptions/" + e.SubscriptionID + "/ResourceGroups/" + e.ResourceGroup,
	}, nil
}
