// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"fmt"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/cli/azure"
	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/Azure/radius/pkg/cli/localrp"
	"github.com/Azure/radius/pkg/cli/server"
	"k8s.io/client-go/dynamic"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// LocalEnvironment represents a local Radius environment
type LocalEnvironment struct {
	Name               string `mapstructure:"name" validate:"required"`
	Kind               string `mapstructure:"kind" validate:"required"`
	Purpose            string `mapstructure:"purpose" yaml:",omitempty"`
	DefaultApplication string `mapstructure:"defaultapplication" yaml:",omitempty"`
	SubscriptionID     string `mapstructure:"subscriptionid" yaml:",omitempty"`
	ResourceGroup      string `mapstructure:"resourcegroup" yaml:",omitempty"`

	// We tolerate and allow extra fields - this helps with forwards compat.
	Properties map[string]interface{} `mapstructure:",remain" yaml:",omitempty"`
}

func (e *LocalEnvironment) GetName() string {
	return e.Name
}

func (e *LocalEnvironment) GetKind() string {
	return e.Kind
}

func (e *LocalEnvironment) GetPurpose() string {
	return e.Purpose
}

func (e *LocalEnvironment) GetDefaultApplication() string {
	return e.DefaultApplication
}

func (e *LocalEnvironment) GetStatusLink() string {
	return ""
}

func (e *LocalEnvironment) HasAzureProvider() bool {
	return e.SubscriptionID != "" && e.ResourceGroup != ""
}

func (e *LocalEnvironment) GetAzureProviderDetails() (string, string) {
	if e.HasAzureProvider() {
		return e.SubscriptionID, e.ResourceGroup
	}

	return "test-subscription", "test-resource-group"
}

func (e *LocalEnvironment) GetURL() string {
	return "http://localhost:9999"
}

func (e *LocalEnvironment) CreateDeploymentClient(ctx context.Context) (clients.DeploymentClient, error) {
	subscriptionID, resourceGroup := e.GetAzureProviderDetails()

	providers := map[string]localrp.DeploymentProvider{
		"radius": {
			Authorizer: nil,
			BaseURL:    e.GetURL(),
			Connection: arm.NewConnection(e.GetURL(), &radclient.AnonymousCredential{}, nil),
		},
	}

	if e.HasAzureProvider() {
		authorizer, err := armauth.GetArmAuthorizer()
		if err != nil {
			return nil, fmt.Errorf("failed to obtain a Azure credentials: %w", err)
		}

		azcred, err := azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to obtain a Azure credentials: %w", err)
		}

		providers["azure"] = localrp.DeploymentProvider{
			Authorizer: authorizer,
			BaseURL:    arm.AzurePublicCloud,
			Connection: arm.NewDefaultConnection(azcred, nil),
		}
	}

	return &localrp.LocalRPDeploymentClient{
		Providers:      providers,
		SubscriptionID: subscriptionID,
		ResourceGroup:  resourceGroup,
	}, nil
}

func (e *LocalEnvironment) CreateDiagnosticsClient(ctx context.Context) (clients.DiagnosticsClient, error) {
	kubeConfig, err := e.GetKubeConfigPath()
	if err != nil {
		return nil, err
	}

	rawconfig, err := clientcmd.LoadFromFile(kubeConfig)
	if err != nil {
		return nil, err
	}

	context := rawconfig.Contexts[rawconfig.CurrentContext]
	if context == nil {
		return nil, fmt.Errorf("kubernetes context '%s' could not be found", rawconfig.CurrentContext)
	}

	clientconfig := clientcmd.NewNonInteractiveClientConfig(*rawconfig, rawconfig.CurrentContext, nil, nil)
	config, err := clientconfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	k8sClient, err := k8s.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	dyn, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	azcred := &radclient.AnonymousCredential{}
	con := arm.NewConnection(e.GetURL(), azcred, nil)

	subscriptionID, resourceGroup := e.GetAzureProviderDetails()

	return &localrp.LocalDiagnosticsClient{
		K8sClient:      k8sClient,
		DynamicClient:  dyn,
		ResourceClient: *radclient.NewRadiusResourceClient(con, "test-subscription"),
		SubscriptionID: subscriptionID,
		ResourceGroup:  resourceGroup,
	}, nil
}

func (e *LocalEnvironment) CreateManagementClient(ctx context.Context) (clients.ManagementClient, error) {
	azcred := &radclient.AnonymousCredential{}
	con := arm.NewConnection(e.GetURL(), azcred, nil)

	subscriptionID, resourceGroup := e.GetAzureProviderDetails()

	return &azure.ARMManagementClient{
		Connection:      con,
		SubscriptionID:  subscriptionID,
		ResourceGroup:   resourceGroup,
		EnvironmentName: e.Name,
	}, nil
}

func (e *LocalEnvironment) GetKubeConfigPath() (string, error) {
	filePath, err := server.GetLocalKubeConfigPath()
	if err != nil {
		return "", err
	}

	_, err = os.Stat(filePath)
	if err == os.ErrNotExist {
		return "", fmt.Errorf("could not find local config. Use rad server run to start server")
	} else if err != nil {
		return "", err
	}

	return filePath, nil
}
