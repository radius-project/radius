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
	Name                       string `mapstructure:"name" validate:"required"`
	Kind                       string `mapstructure:"kind" validate:"required"`
	SubscriptionID             string `mapstructure:"subscriptionid" validate:"required"`
	ResourceGroup              string `mapstructure:"resourcegroup" validate:"required"`
	ClusterName                string `mapstructure:"clustername" validate:"required"`
	DefaultApplication         string `mapstructure:"defaultapplication" yaml:",omitempty"`
	Context                    string `mapstructure:"context" validate:"required"`
	Namespace                  string `mapstructure:"namespace" validate:"required"`
	APIServerBaseURL           string `mapstructure:"apiserverbaseurl,omitempty"`
	APIDeploymentEngineBaseURL string `mapstructure:"apideploymentenginebaseurl,omitempty"`

	// We tolerate and allow extra fields - this helps with forwards compat.
	Properties map[string]interface{} `mapstructure:",remain" yaml:",omitempty"`
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

func (e *AzureCloudEnvironment) GetContainerRegistry() *Registry {
	return nil
}

func (e *AzureCloudEnvironment) GetStatusLink() string {
	// If there's a problem generating the status link, we don't want to fail noisily, just skip the link.
	url, err := azure.GenerateAzureEnvUrl(e.SubscriptionID, e.ResourceGroup)
	if err != nil {
		return ""
	}

	return url
}

func (e *AzureCloudEnvironment) CreateDeploymentClient(ctx context.Context) (clients.DeploymentClient, error) {
	url, roundTripper, err := kubernetes.GetBaseUrlAndRoundTripperForDeploymentEngine(e.APIDeploymentEngineBaseURL, e.Context)
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

	dc := azclients.NewDeploymentsClientWithBaseURI(url, e.SubscriptionID)

	// Poll faster than the default, many deployments are quick
	dc.PollingDelay = 5 * time.Second

	dc.Sender = &sender{RoundTripper: roundTripper}

	op := azclients.NewOperationsClientWithBaseUri(url, e.SubscriptionID)
	op.PollingDelay = 5 * time.Second
	op.Sender = &sender{RoundTripper: roundTripper}

	return &azure.ARMDeploymentClient{
		Client:           dc,
		OperationsClient: op,
		SubscriptionID:   e.SubscriptionID,
		ResourceGroup:    e.ResourceGroup,
		Tags:             tags,
	}, nil
}

func (e *AzureCloudEnvironment) CreateDiagnosticsClient(ctx context.Context) (clients.DiagnosticsClient, error) {
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

	return &azure.ARMDiagnosticsClient{
		K8sClient:      k8sClient,
		RestConfig:     config,
		Client:         client,
		ResourceClient: *radclient.NewRadiusResourceClient(con, e.SubscriptionID),
		ResourceGroup:  e.ResourceGroup,
		SubscriptionID: e.SubscriptionID,
	}, nil
}

func (e *AzureCloudEnvironment) CreateManagementClient(ctx context.Context) (clients.ManagementClient, error) {
	_, connection, err := kubernetes.CreateAPIServerConnection(e.Context, e.APIServerBaseURL)
	if err != nil {
		return nil, err
	}

	return &azure.ARMManagementClient{
		EnvironmentName: e.Name,
		Connection:      connection,
		ResourceGroup:   e.ResourceGroup,
		SubscriptionID:  e.SubscriptionID,
	}, nil
}
