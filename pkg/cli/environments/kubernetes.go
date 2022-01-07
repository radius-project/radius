// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/cli/azure"
	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/Azure/radius/pkg/cli/kubernetes"
)

// KubernetesEnvironment represents a Kubernetes Radius environment.
type KubernetesEnvironment struct {
	Name               string `mapstructure:"name" validate:"required"`
	Kind               string `mapstructure:"kind" validate:"required"`
	Context            string `mapstructure:"context" validate:"required"`
	Namespace          string `mapstructure:"namespace" validate:"required"`
	DefaultApplication string `mapstructure:"defaultapplication,omitempty"`
	ApiServerBaseURL   string `mapstructure:"apiserverbaseurl,omitempty"`

	// We tolerate and allow extra fields - this helps with forwards compat.
	Properties map[string]interface{} `mapstructure:",remain"`
}

func (e *KubernetesEnvironment) GetName() string {
	return e.Name
}

func (e *KubernetesEnvironment) GetKind() string {
	return e.Kind
}

func (e *KubernetesEnvironment) GetDefaultApplication() string {
	return e.DefaultApplication
}

// No Status Link for kubernetes
func (e *KubernetesEnvironment) GetStatusLink() string {
	return ""
}

func (e *KubernetesEnvironment) CreateDeploymentClient(ctx context.Context) (clients.DeploymentClient, error) {
	client, err := kubernetes.CreateRuntimeClient(e.Context, kubernetes.Scheme)
	if err != nil {
		return nil, err
	}
	dynamicClient, err := kubernetes.CreateDynamicClient(e.Context)
	if err != nil {
		return nil, err
	}
	typedClient, _, err := kubernetes.CreateTypedClient(e.Context)
	if err != nil {
		return nil, err
	}

	return &kubernetes.KubernetesDeploymentClient{
		Client:    client,
		Dynamic:   dynamicClient,
		Typed:     typedClient,
		Namespace: e.Namespace,
	}, nil
}

func (e *KubernetesEnvironment) CreateDiagnosticsClient(ctx context.Context) (clients.DiagnosticsClient, error) {
	k8sClient, config, err := kubernetes.CreateTypedClient(e.Context)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.CreateRuntimeClient(e.Context, kubernetes.Scheme)
	if err != nil {
		return nil, err
	}

	return &kubernetes.KubernetesDiagnosticsClient{
		K8sClient:  k8sClient,
		Client:     client,
		RestConfig: config,
		Namespace:  e.Namespace,
	}, nil
}

func (e *KubernetesEnvironment) CreateManagementClient(ctx context.Context) (clients.ManagementClient, error) {

	restConfig, err := kubernetes.CreateRestConfig(e.Context)
	if err != nil {
		return nil, err
	}

	roundTripper, err := kubernetes.CreateRestRoundTripper(e.Context)
	if err != nil {
		return nil, err
	}
	azcred := &radclient.AnonymousCredential{}

	// ApiServerBaseURL will change the
	var connection *arm.Connection
	if e.ApiServerBaseURL != "" {
		connection = arm.NewConnection(fmt.Sprintf("%s/apis/api.radius.dev/v1alpha3", e.ApiServerBaseURL), azcred, &arm.ConnectionOptions{})
	} else {
		connection = arm.NewConnection(fmt.Sprintf("%s/apis/api.radius.dev/v1alpha3", restConfig.Host+restConfig.APIPath), azcred, &arm.ConnectionOptions{
			HTTPClient: &KubernetesRoundTripper{Client: roundTripper},
		})
	}

	return &azure.ARMManagementClient{
		EnvironmentName: e.Name,
		Connection:      connection,
		ResourceGroup:   e.Namespace, // Temporarily set resource group and subscription id to the namespace
		SubscriptionID:  e.Namespace,
	}, nil
}

var _ policy.Transporter = &KubernetesRoundTripper{}

type KubernetesRoundTripper struct {
	Client http.RoundTripper
}

func (t *KubernetesRoundTripper) Do(req *http.Request) (*http.Response, error) {
	resp, err := t.Client.RoundTrip(req)
	return resp, err
}
