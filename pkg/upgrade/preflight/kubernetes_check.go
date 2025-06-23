/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package preflight

import (
	"context"
	"fmt"

	"github.com/radius-project/radius/pkg/kubeutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// RadiusSystemNamespace is the namespace where Radius components are installed
	RadiusSystemNamespace = "radius-system"
)

// Ensure KubernetesConnectivityCheck implements PreflightCheck interface
var _ PreflightCheck = (*KubernetesConnectivityCheck)(nil)

// KubernetesConnectivityCheck validates cluster connectivity and basic permissions.
type KubernetesConnectivityCheck struct {
	kubeContext string
	clientset   kubernetes.Interface
}

// NewKubernetesConnectivityCheck creates a new check that will create its own client.
func NewKubernetesConnectivityCheck(kubeContext string) *KubernetesConnectivityCheck {
	return &KubernetesConnectivityCheck{
		kubeContext: kubeContext,
	}
}

// NewKubernetesConnectivityCheckWithClientset creates a new check with an existing client.
func NewKubernetesConnectivityCheckWithClientset(kubeContext string, clientset kubernetes.Interface) *KubernetesConnectivityCheck {
	return &KubernetesConnectivityCheck{
		kubeContext: kubeContext,
		clientset:   clientset,
	}
}

// Name returns the name of this check.
func (k *KubernetesConnectivityCheck) Name() string {
	return "Kubernetes Connectivity"
}

// Severity returns the severity level of this check.
func (k *KubernetesConnectivityCheck) Severity() CheckSeverity {
	return SeverityError
}

// Run executes the connectivity check.
func (k *KubernetesConnectivityCheck) Run(ctx context.Context) (bool, string, error) {
	clientset, err := k.getClientset()
	if err != nil {
		return false, "", err
	}

	// Check basic connectivity
	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return false, "Cannot connect to Kubernetes cluster", fmt.Errorf("failed to get server version: %w", err)
	}

	// Check namespace existence - required for upgrade
	_, err = clientset.CoreV1().Namespaces().Get(ctx, RadiusSystemNamespace, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Sprintf("Connected (version: %s) but %s namespace not found", serverVersion.GitVersion, RadiusSystemNamespace), nil
	}

	// Check deployment permissions
	_, err = clientset.AppsV1().Deployments(RadiusSystemNamespace).List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return false, fmt.Sprintf("Connected (version: %s) but insufficient permissions", serverVersion.GitVersion), nil
	}

	return true, fmt.Sprintf("Connected (version: %s) with sufficient permissions", serverVersion.GitVersion), nil
}

// getClientset returns the clientset, creating one if necessary.
func (k *KubernetesConnectivityCheck) getClientset() (kubernetes.Interface, error) {
	if k.clientset != nil {
		return k.clientset, nil
	}

	config, err := kubeutil.NewClientConfig(&kubeutil.ConfigOptions{
		ContextName: k.kubeContext,
		QPS:         kubeutil.DefaultCLIQPS,
		Burst:       kubeutil.DefaultCLIBurst,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return clientset, nil
}
