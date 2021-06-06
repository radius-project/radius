// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/radius/pkg/rad/azure"
	"github.com/Azure/radius/pkg/rad/clients"
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
	Name               string `mapstructure:"name" validate:"required"`
	Kind               string `mapstructure:"kind" validate:"required"`
	SubscriptionID     string `mapstructure:"subscriptionid" validate:"required"`
	ResourceGroup      string `mapstructure:"resourcegroup" validate:"required"`
	ClusterName        string `mapstructure:"clustername" validate:"required"`
	DefaultApplication string `mapstructure:"defaultapplication,omitempty"`

	// We tolerate and allow extra fields - this helps with forwards compat.
	Properties map[string]interface{} `mapstructure:",remain"`
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

func (e *AzureCloudEnvironment) CreateDeploymentClient() (clients.DeploymentClient, error) {
	dc := resources.NewDeploymentsClient(e.SubscriptionID)
	armauth, err := azure.GetResourceManagerEndpointAuthorizer()
	if err != nil {
		return nil, err
	}

	dc.Authorizer = armauth

	// Poll faster than the default, many deployments are quick
	dc.PollingDelay = 5 * time.Second

	// Don't timeout, let the user cancel
	dc.PollingDuration = 0

	return &azure.ARMDeploymentClient{
		Client:         dc,
		SubscriptionID: e.SubscriptionID,
		ResourceGroup:  e.ResourceGroup,
	}, nil
}

func (e *AzureCloudEnvironment) CreateDiagnosticsClient() (clients.DiagnosticsClient, error) {
	return &azure.ARMDiagnosticsClient{}, nil
}

func (e *AzureCloudEnvironment) CreateManagementClient() (clients.ManagementClient, error) {
	return &azure.ARMManagementClient{}, nil
}
