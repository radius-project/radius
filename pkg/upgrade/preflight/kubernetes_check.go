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

// KubernetesConnectivityCheck validates that kubectl can connect to the cluster
// and that the user has sufficient permissions to perform upgrade operations.
type KubernetesConnectivityCheck struct {
	kubeContext string
}

// NewKubernetesConnectivityCheck creates a new Kubernetes connectivity check.
func NewKubernetesConnectivityCheck(kubeContext string) *KubernetesConnectivityCheck {
	return &KubernetesConnectivityCheck{
		kubeContext: kubeContext,
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

// Run executes the Kubernetes connectivity check.
func (k *KubernetesConnectivityCheck) Run(ctx context.Context) (bool, string, error) {
	// Create Kubernetes client config
	config, err := kubeutil.NewClientConfig(&kubeutil.ConfigOptions{
		ContextName: k.kubeContext,
		QPS:         kubeutil.DefaultCLIQPS,
		Burst:       kubeutil.DefaultCLIBurst,
	})
	if err != nil {
		return false, "", fmt.Errorf("failed to create Kubernetes client config: %w", err)
	}

	// Create Kubernetes clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return false, "", fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Test basic connectivity by trying to get cluster version
	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return false, "Cannot connect to Kubernetes cluster", fmt.Errorf("failed to get server version: %w", err)
	}

	_, err = clientset.CoreV1().Namespaces().Get(ctx, "radius-system", metav1.GetOptions{})
	if err != nil {
		return true, fmt.Sprintf("Connected to Kubernetes cluster (version: %s), radius-system namespace not found", serverVersion.String()), nil
	}

	_, err = clientset.AppsV1().Deployments("radius-system").List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return false, fmt.Sprintf("Connected to Kubernetes cluster (version: %s) but insufficient permissions to list deployments in radius-system namespace", serverVersion.String()), nil
	}

	return true, fmt.Sprintf("Successfully connected to Kubernetes cluster (version: %s) with sufficient permissions", serverVersion.String()), nil
}
