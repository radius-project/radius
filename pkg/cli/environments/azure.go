// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/radius/pkg/azure/aks"
	"github.com/Azure/radius/pkg/azure/armauth"
	azclients "github.com/Azure/radius/pkg/azure/clients"
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/cli/azure"
	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/Azure/radius/pkg/cli/kubernetes"
	k8s "k8s.io/client-go/kubernetes"
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
	Name                      string `mapstructure:"name" validate:"required"`
	Kind                      string `mapstructure:"kind" validate:"required"`
	SubscriptionID            string `mapstructure:"subscriptionid" validate:"required"`
	ResourceGroup             string `mapstructure:"resourcegroup" validate:"required"`
	ControlPlaneResourceGroup string `mapstring:"controlplaneresourcegroup" validate:"required"`
	ClusterName               string `mapstructure:"clustername" validate:"required"`
	DefaultApplication        string `mapstructure:"defaultapplication" yaml:",omitempty"`

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

func (e *AzureCloudEnvironment) GetStatusLink() string {
	// If there's a problem generating the status link, we don't want to fail noisily, just skip the link.
	url, err := azure.GenerateAzureEnvUrl(e.SubscriptionID, e.ResourceGroup)
	if err != nil {
		return ""
	}

	return url
}

func (e *AzureCloudEnvironment) CreateDeploymentClient(ctx context.Context) (clients.DeploymentClient, error) {
	armauth, err := armauth.GetArmAuthorizer()
	if err != nil {
		return nil, err
	}

	dc := azclients.NewDeploymentsClient(e.SubscriptionID, armauth)
	// Poll faster than the default, many deployments are quick
	dc.PollingDelay = 5 * time.Second

	opc := azclients.NewDeploymentOperationsClient(e.SubscriptionID, armauth)

	return &azure.ARMDeploymentClient{
		DeploymentsClient: dc,
		OperationsClient:  opc,
		SubscriptionID:    e.SubscriptionID,
		ResourceGroup:     e.ResourceGroup,
	}, nil
}

func (e *AzureCloudEnvironment) CreateDiagnosticsClient(ctx context.Context) (clients.DiagnosticsClient, error) {
	config, err := aks.GetAKSMonitoringCredentials(ctx, e.SubscriptionID, e.ControlPlaneResourceGroup, e.ClusterName)
	if err != nil {
		return nil, err
	}

	k8sClient, err := k8s.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	azcred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain a Azure credentials: %w", err)
	}

	con := arm.NewDefaultConnection(azcred, nil)

	return &azure.AKSDiagnosticsClient{
		KubernetesDiagnosticsClient: kubernetes.KubernetesDiagnosticsClient{
			Client:     k8sClient,
			RestConfig: config,
		},
		SubscriptionID: e.SubscriptionID,
		ResourceGroup:  e.ResourceGroup,
		ResourceClient: *radclient.NewRadiusResourceClient(con, e.SubscriptionID),
	}, nil
}

func (e *AzureCloudEnvironment) CreateManagementClient(ctx context.Context) (clients.ManagementClient, error) {
	azcred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain a Azure credentials: %w", err)
	}

	con := arm.NewDefaultConnection(azcred, nil)

	return &azure.ARMManagementClient{
		Connection:      con,
		ResourceGroup:   e.ResourceGroup,
		SubscriptionID:  e.SubscriptionID,
		EnvironmentName: e.Name}, nil
}
