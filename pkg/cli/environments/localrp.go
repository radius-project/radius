// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/project-radius/radius/pkg/azure/armauth"
	azclients "github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/localrp"
)

// LocalRPEnvironment represents a local test setup for Azure Cloud Radius environment.
type LocalRPEnvironment struct {
	Name                      string `mapstructure:"name" validate:"required"`
	Kind                      string `mapstructure:"kind" validate:"required"`
	SubscriptionID            string `mapstructure:"subscriptionid" validate:"required"`
	ResourceGroup             string `mapstructure:"resourcegroup" validate:"required"`
	ControlPlaneResourceGroup string `mapstring:"controlplaneresourcegroup" validate:"required"`
	ClusterName               string `mapstructure:"clustername" validate:"required"`
	DefaultApplication        string `mapstructure:"defaultapplication,omitempty"`
	Context                   string `mapstructure:"context" validate:"required"`
	Namespace                 string `mapstructure:"namespace" validate:"required"`

	// URL for the Deployment Engine, TODO run this as part of the start of a deployment
	// if no URL is provided.
	APIDeploymentEngineBaseURL string `mapstructure:"apideploymentenginebaseurl"`

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

func (e *LocalRPEnvironment) GetDefaultApplication() string {
	return e.DefaultApplication
}

func (e *LocalRPEnvironment) GetContainerRegistry() *Registry {
	return nil
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
	var deUrl string
	var bindUrl string

	if e.APIDeploymentEngineBaseURL == "" {
		// Bind to a random port on localhost
		// There is a slight delay between getting a port and then
		// the deployment engine binding to it, so hopefully this
		// is reliable enough.
		listener, err := net.Listen("tcp", ":0")
		if err != nil {
			return nil, err
		}
		port := listener.Addr().(*net.TCPAddr).Port
		err = listener.Close()
		if err != nil {
			return nil, err
		}
		bindUrl = fmt.Sprintf("http://localhost:%d", port)
		deUrl = kubernetes.GetBaseUrlForDeploymentEngine(bindUrl)
	} else {
		deUrl = kubernetes.GetBaseUrlForDeploymentEngine(e.APIDeploymentEngineBaseURL)
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

	dc := azclients.NewDeploymentsClientWithBaseURI(deUrl, e.SubscriptionID)

	// Poll faster than the default, many deployments are quick
	dc.PollingDelay = 5 * time.Second
	dc.Authorizer = auth

	op := azclients.NewOperationsClientWithBaseUri(deUrl, e.SubscriptionID)
	op.PollingDelay = 5 * time.Second
	op.Authorizer = auth

	client := &localrp.LocalRPDeploymentClient{
		InnerClient: azure.ARMDeploymentClient{
			Client:           dc,
			OperationsClient: op,
			SubscriptionID:   e.SubscriptionID,
			ResourceGroup:    e.ResourceGroup,
			Tags:             tags,
		},
		BindUrl:    bindUrl,
		BackendUrl: e.URL,
	}

	return client, nil
}

func (e *LocalRPEnvironment) CreateDiagnosticsClient(ctx context.Context) (clients.DiagnosticsClient, error) {
	k8sClient, config, err := kubernetes.CreateTypedClient(e.Context)
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.CreateRuntimeClient(e.Context, kubernetes.Scheme)
	if err != nil {
		return nil, err
	}

	azcred := &radclient.AnonymousCredential{}
	con := arm.NewConnection(e.URL, azcred, nil)

	return &azure.ARMDiagnosticsClient{
		K8sClient:      k8sClient,
		RestConfig:     config,
		Client:         client,
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
