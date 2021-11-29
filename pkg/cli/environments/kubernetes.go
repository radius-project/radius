// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
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

	restClient, err := kubernetes.CreateRestConfig(e.Context)
	if err != nil {
		return nil, err
	}

	azcred := &K8sToken{restClient.BearerToken}
	// 	con := arm.NewDefaultConnection(azcred, nil)
	connection := arm.NewConnection(fmt.Sprintf("%s%s%s", restClient.Host, restClient.APIPath, "/apis/api.radius.dev/v1alpha3"), azcred, nil)

	return &kubernetes.KubernetesManagementClient{
		Client:          client,
		DynamicClient:   dynamicClient,
		ExtensionClient: extensionClient,
		Namespace:       e.Namespace,
		EnvironmentName: e.Name,
		Connection:      connection,
		ResourceGroup:   "test", // TODO fill these in with more specific info about env
		SubscriptionID:  "123",
	}, nil
}

var _ azcore.TokenCredential = &K8sToken{}

type K8sToken struct {
	BearerToken string
}

func (k *K8sToken) GetToken(ctx context.Context, options policy.TokenRequestOptions) (*azcore.AccessToken, error) {
	return &azcore.AccessToken{Token: k.BearerToken}, nil
}

// PolicyFunc is a type that implements the Policy interface.
// Use this type when implementing a stateless policy as a first-class function.
type PolicyFunc func(*policy.Request) (*http.Response, error)

// Do implements the Policy interface on PolicyFunc.
func (pf PolicyFunc) Do(req *policy.Request) (*http.Response, error) {
	return pf(req)
}

func (*K8sToken) NewAuthenticationPolicy(options runtime.AuthenticationOptions) policy.Policy {
	return PolicyFunc(func(req *policy.Request) (*http.Response, error) {
		return req.Next()
	})
}

// var _ azcore.TokenCredential = &AnonymousCredential{}

// type AnonymousCredential struct {
// }

// // PolicyFunc is a type that implements the Policy interface.
// // Use this type when implementing a stateless policy as a first-class function.
// type PolicyFunc func(*policy.Request) (*http.Response, error)

// // Do implements the Policy interface on PolicyFunc.
// func (pf PolicyFunc) Do(req *policy.Request) (*http.Response, error) {
// 	return pf(req)
// }

// func (*AnonymousCredential) NewAuthenticationPolicy(options runtime.AuthenticationOptions) policy.Policy {
// 	return PolicyFunc(func(req *policy.Request) (*http.Response, error) {
// 		return req.Next()
// 	})
// }

// func (a *AnonymousCredential) GetToken(ctx context.Context, options policy.TokenRequestOptions) (*azcore.AccessToken, error) {
// 	return nil, nil
// }
