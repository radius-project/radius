// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerservice/mgmt/containerservice"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/sdk/armcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/radius/pkg/rad/azure"
	"github.com/Azure/radius/pkg/rad/clients"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
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

func (e *AzureCloudEnvironment) CreateDiagnosticsClient(ctx context.Context) (clients.DiagnosticsClient, error) {
	config, err := e.getMonitoringCredentials(ctx)
	if err != nil {
		return nil, err
	}

	k8sClient, err := k8s.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &azure.ARMDiagnosticsClient{
		Client:     k8sClient,
		RestConfig: config,
	}, nil
}

func (e *AzureCloudEnvironment) CreateManagementClient() (clients.ManagementClient, error) {
	azcred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain a Azure credentials: %w", err)
	}

	con := armcore.NewDefaultConnection(azcred, nil)

	return &azure.ARMManagementClient{
		AzCred:         azcred,
		Connection:     con,
		ResourceGroup:  e.ResourceGroup,
		SubscriptionID: e.SubscriptionID}, nil
}

func (e *AzureCloudEnvironment) getMonitoringCredentials(ctx context.Context) (*rest.Config, error) {
	armauth, err := azure.GetResourceManagerEndpointAuthorizer()
	if err != nil {
		return nil, err
	}

	// Currently we go to AKS every time to ask for credentials, we don't
	// cache them locally. This could be done in the future, but skipping it for now
	// since it's non-obvious that we'd store credentials in your ~/.rad directory
	mcc := containerservice.NewManagedClustersClient(e.SubscriptionID)
	mcc.Authorizer = armauth

	results, err := mcc.ListClusterMonitoringUserCredentials(ctx, e.ResourceGroup, e.ClusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to list AKS cluster credentials: %w", err)
	}

	if results.Kubeconfigs == nil || len(*results.Kubeconfigs) == 0 {
		return nil, errors.New("failed to list AKS cluster credentials: response did not contain credentials")
	}

	kc := (*results.Kubeconfigs)[0]
	c, err := clientcmd.NewClientConfigFromBytes(*kc.Value)
	if err != nil {
		return nil, fmt.Errorf("kubeconfig was invalid: %w", err)
	}

	restconfig, err := c.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("kubeconfig did not contain client credentials: %w", err)
	}

	return restconfig, nil
}
