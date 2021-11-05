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
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/cli/azure"
	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/Azure/radius/pkg/cli/kubernetes"
	"github.com/Azure/radius/pkg/cli/localrp"
	"github.com/Azure/radius/pkg/cli/server"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// LocalEnvironment represents a local Radius environment
type LocalEnvironment struct {
	Name               string `mapstructure:"name" validate:"required"`
	Kind               string `mapstructure:"kind" validate:"required"`
	DefaultApplication string `mapstructure:"defaultapplication" yaml:",omitempty"`

	// We tolerate and allow extra fields - this helps with forwards compat.
	Properties map[string]interface{} `mapstructure:",remain" yaml:",omitempty"`
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

func (e *LocalEnvironment) CreateDeploymentClient(ctx context.Context) (clients.DeploymentClient, error) {
	azcred := &radclient.AnonymousCredential{}
	connection := arm.NewConnection("http://localhost:9999", azcred, nil)

	return &localrp.LocalRPDeploymentClient{
		Authorizer:     nil,
		BaseURL:        "http://localhost:9999",
		Connection:     connection,
		SubscriptionID: "test-subscription",
		ResourceGroup:  "test-resource-group",
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

	azcred := &radclient.AnonymousCredential{}
	con := arm.NewConnection("http://localhost:9999", azcred, nil)

	return &azure.AKSDiagnosticsClient{
		KubernetesDiagnosticsClient: kubernetes.KubernetesDiagnosticsClient{
			Client:     k8sClient,
			RestConfig: config,
		},
		ResourceClient: *radclient.NewRadiusResourceClient(con, "test-subscription"),
		SubscriptionID: "test-subscription",
		ResourceGroup:  "test-resource-group",
	}, nil
}

func (e *LocalEnvironment) CreateManagementClient(ctx context.Context) (clients.ManagementClient, error) {
	azcred := &radclient.AnonymousCredential{}
	con := arm.NewConnection("http://localhost:9999", azcred, nil)

	return &azure.ARMManagementClient{
		Connection:      con,
		SubscriptionID:  "test-subscription",
		ResourceGroup:   "test-resource-group",
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
