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
	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/Azure/radius/pkg/cli/kubernetes"
)

const (
	headerXmsDate                = "x-ms-date"
	headerUserAgent              = "User-Agent"
	headerURLEncoded             = "application/x-www-form-urlencoded"
	headerAuthorization          = "Authorization"
	headerAuxiliaryAuthorization = "x-ms-authorization-auxiliary"
	headerMetadata               = "Metadata"
	headerContentType            = "Content-Type"
	bearerTokenPrefix            = "Bearer "
)

// KubernetesEnvironment represents a Kubernetes Radius environment.
type KubernetesEnvironment struct {
	Name               string `mapstructure:"name" validate:"required"`
	Kind               string `mapstructure:"kind" validate:"required"`
	Context            string `mapstructure:"context" validate:"required"`
	Namespace          string `mapstructure:"namespace" validate:"required"`
	DefaultApplication string `mapstructure:"defaultapplication,omitempty"`

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
	// azcred := &radclient.AnonymousCredential{}
	// connection := arm.NewConnection("http://localhost:9999", azcred, nil)

	// return &kubernetes.KubernetesDeploymentClient{
	// 	Client:    client,
	// 	Namespace: e.Namespace,
	// }, nil
	client, err := kubernetes.CreateRuntimeClient(e.Context, kubernetes.Scheme)
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
	client, config, err := kubernetes.CreateTypedClient(e.Context)
	if err != nil {
		return nil, err
	}

	return &kubernetes.KubernetesDiagnosticsClient{
		Client:     client,
		RestConfig: config,
		Namespace:  e.Namespace,
	}, nil
}

func (e *KubernetesEnvironment) CreateManagementClient(ctx context.Context) (clients.ManagementClient, error) {
	client, err := kubernetes.CreateRuntimeClient(e.Context, kubernetes.Scheme)
	if err != nil {
		return nil, err
	}
	dynamicClient, err := kubernetes.CreateDynamicClient(e.Context)
	if err != nil {
		return nil, err
	}
	extensionClient, err := kubernetes.CreateExtensionClient(e.Context)
	if err != nil {
		return nil, err
	}

	restConfig, err := kubernetes.CreateRestConfig(e.Context)
	if err != nil {
		return nil, err
	}

	roundTripper, err := kubernetes.CreateRestRoundTripper(e.Context)
	if err != nil {
		return nil, err
	}

	azcred := &radclient.AnonymousCredential{}

	connection := arm.NewConnection(fmt.Sprintf("%s%s%s", restConfig.Host, restConfig.APIPath, "/apis/api.radius.dev/v1alpha3"), azcred, &arm.ConnectionOptions{
		HTTPClient: &TestClient{Client: roundTripper},
	})
	return &kubernetes.KubernetesManagementClient{
		Client:          client,
		DynamicClient:   dynamicClient,
		ExtensionClient: extensionClient,
		Namespace:       e.Namespace,
		EnvironmentName: e.Name,
		Connection:      connection,
		ResourceGroup:   e.Namespace, // TODO fill these in with more specific info about env
		SubscriptionID:  restConfig.Host,
	}, nil
}

var _ policy.Transporter = &TestClient{}

type TestClient struct {
	Client http.RoundTripper
}

func (t *TestClient) Do(req *http.Request) (*http.Response, error) {
	resp, err := t.Client.RoundTrip(req)
	return resp, err
}
