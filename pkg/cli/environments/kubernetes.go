// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"

	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
)

// KubernetesEnvironment represents a Kubernetes Radius environment.
type KubernetesEnvironment struct {
	Name               string `mapstructure:"name" validate:"required"`
	Kind               string `mapstructure:"kind" validate:"required"`
	Context            string `mapstructure:"context" validate:"required"`
	Namespace          string `mapstructure:"namespace" validate:"required"`
	DefaultApplication string `mapstructure:"defaultapplication,omitempty"`
	APIServerBaseURL   string `mapstructure:"apiserverbaseurl,omitempty"`

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

func (e *KubernetesEnvironment) GetContainerRegistry() *Registry {
	return nil
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
	_, connection, err := kubernetes.CreateAPIServerConnection(e.Context, e.APIServerBaseURL)
	if err != nil {
		return nil, err
	}

	return &azure.ARMManagementClient{
		EnvironmentName: e.Name,
		Connection:      connection,
		ResourceGroup:   e.Namespace, // Temporarily set resource group and subscription id to the namespace
		SubscriptionID:  e.Namespace,
	}, nil
}
