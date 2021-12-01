// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"

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
	return &kubernetes.KubernetesManagementClient{
		Client:          client,
		DynamicClient:   dynamicClient,
		ExtensionClient: extensionClient,
		Namespace:       e.Namespace,
		EnvironmentName: e.Name,
	}, nil
}
