// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/radius/pkg/azure/aks"
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/cli/azure"
	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/Azure/radius/pkg/cli/kubernetes"
	"github.com/Azure/radius/pkg/cli/localrp"
	k8s "k8s.io/client-go/kubernetes"
)

// LocalRPEnvironment represents a local test setup for Azure Cloud Radius environment.
type LocalRPEnvironment struct {
	Name                      string `mapstructure:"name" validate:"required"`
	Kind                      string `mapstructure:"kind" validate:"required"`
	Purpose                   string `mapstructure:"purpose" yaml:",omitempty"`
	SubscriptionID            string `mapstructure:"subscriptionid" validate:"required"`
	ResourceGroup             string `mapstructure:"resourcegroup" validate:"required"`
	ControlPlaneResourceGroup string `mapstring:"controlplaneresourcegroup" validate:"required"`
	ClusterName               string `mapstructure:"clustername" validate:"required"`
	DefaultApplication        string `mapstructure:"defaultapplication,omitempty"`

	// URL for the local RP
	URL string `mapstructure:"url,omitempty" validate:"required"`

	// We tolerate and allow extra fields - this helps with forwards compat.
	Properties map[string]interface{} `mapstructure:",remain"`
}

func (e *LocalRPEnvironment) GetName() string {
	return e.Name
}

func (e *LocalRPEnvironment) GetKind() string {
	return e.Kind
}

func (e *LocalRPEnvironment) GetPurpose() string {
	return e.Purpose
}

func (e *LocalRPEnvironment) GetDefaultApplication() string {
	return e.DefaultApplication
}

func (e *LocalRPEnvironment) GetStatusLink() string {
	// If there's a problem generating the status link, we don't want to fail noisily, just skip the link.
	url, err := azure.GenerateAzureEnvUrl(e.SubscriptionID, e.ResourceGroup)
	if err != nil {
		return ""
	}

	return url
}

func (e *LocalRPEnvironment) CreateDeploymentClient(ctx context.Context) (clients.DeploymentClient, error) {
	authorizer, err := armauth.GetArmAuthorizer()
	if err != nil {
		return nil, fmt.Errorf("failed to obtain a Azure credentials: %w", err)
	}

	azcred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain a Azure credentials: %w", err)
	}

	providers := map[string]localrp.DeploymentProvider{
		"azure": {
			Authorizer: authorizer,
			BaseURL:    arm.AzurePublicCloud,
			Connection: arm.NewDefaultConnection(azcred, nil),
		},
		"radius": {
			Authorizer: nil,
			BaseURL:    e.URL,
			Connection: arm.NewConnection(e.URL, &radclient.AnonymousCredential{}, nil),
		},
	}

	return &localrp.LocalRPDeploymentClient{
		Providers:      providers,
		SubscriptionID: e.SubscriptionID,
		ResourceGroup:  e.ResourceGroup,
	}, nil
}

func (e *LocalRPEnvironment) CreateDiagnosticsClient(ctx context.Context) (clients.DiagnosticsClient, error) {
	config, err := aks.GetAKSMonitoringCredentials(ctx, e.SubscriptionID, e.ControlPlaneResourceGroup, e.ClusterName)
	if err != nil {
		return nil, err
	}

	k8sClient, err := k8s.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	azcred := &radclient.AnonymousCredential{}
	con := arm.NewConnection(e.URL, azcred, nil)

	return &azure.AKSDiagnosticsClient{
		KubernetesDiagnosticsClient: kubernetes.KubernetesDiagnosticsClient{
			Client:     k8sClient,
			RestConfig: config,
		},
		ResourceClient: *radclient.NewRadiusResourceClient(con, e.SubscriptionID),
		ResourceGroup:  e.ResourceGroup,
		SubscriptionID: e.SubscriptionID,
	}, nil
}

func (e *LocalRPEnvironment) CreateManagementClient(ctx context.Context) (clients.ManagementClient, error) {
	azcred := &radclient.AnonymousCredential{}
	con := arm.NewConnection(e.URL, azcred, nil)

	return &azure.ARMManagementClient{
		Connection:      con,
		ResourceGroup:   e.ResourceGroup,
		SubscriptionID:  e.SubscriptionID,
		EnvironmentName: e.Name,
	}, nil
}
