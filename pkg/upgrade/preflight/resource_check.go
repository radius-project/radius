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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// KubernetesResourceCheck validates that the cluster has sufficient resources
// (CPU, memory) to perform a rolling upgrade of Radius components.
type KubernetesResourceCheck struct {
	kubeContext string
}

// NewKubernetesResourceCheck creates a new Kubernetes resource check.
func NewKubernetesResourceCheck(kubeContext string) *KubernetesResourceCheck {
	return &KubernetesResourceCheck{
		kubeContext: kubeContext,
	}
}

// Name returns the name of this check.
func (k *KubernetesResourceCheck) Name() string {
	return "Kubernetes Resource Availability"
}

// Severity returns the severity level of this check.
func (k *KubernetesResourceCheck) Severity() CheckSeverity {
	return SeverityWarning // Warning level since this is an estimate
}

// Run executes the Kubernetes resource availability check.
func (k *KubernetesResourceCheck) Run(ctx context.Context) (bool, string, error) {
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

	// Get cluster resource availability
	_, totalAllocatable, err := k.getClusterResources(ctx, clientset)
	if err != nil {
		return false, "", fmt.Errorf("failed to get cluster resources: %w", err)
	}

	// Get current Radius resource usage
	radiusUsage, err := k.getCurrentRadiusResourceUsage(ctx, clientset)
	if err != nil {
		// If we can't get current usage, we'll estimate based on defaults
		radiusUsage = k.getEstimatedRadiusResourceUsage()
	}

	// Calculate if we have enough resources for a rolling upgrade
	// During rolling upgrade, we temporarily need ~1.5x the current Radius resources
	upgradeExtraNeeds := ResourceQuota{
		CPU:    radiusUsage.CPU / 2, // Additional 50% during rolling upgrade
		Memory: radiusUsage.Memory / 2,
	}

	// Check if allocatable resources can handle the additional load
	cpuSufficient := totalAllocatable.CPU >= upgradeExtraNeeds.CPU
	memorySufficient := totalAllocatable.Memory >= upgradeExtraNeeds.Memory

	// Format resource values for display
	radiusCPU := float64(radiusUsage.CPU) / 1000.0 // Convert milliCPU to CPU
	radiusMemoryMB := radiusUsage.Memory / (1024 * 1024)
	extraCPU := float64(upgradeExtraNeeds.CPU) / 1000.0
	extraMemoryMB := upgradeExtraNeeds.Memory / (1024 * 1024)
	allocatableCPU := float64(totalAllocatable.CPU) / 1000.0
	allocatableMemoryMB := totalAllocatable.Memory / (1024 * 1024)

	if !cpuSufficient || !memorySufficient {
		return false, fmt.Sprintf(
			"Insufficient resources for upgrade. Current Radius usage: %.2f CPU, %d MB memory. "+
				"Upgrade needs additional: %.2f CPU, %d MB memory. "+
				"Available: %.2f CPU, %d MB memory",
			radiusCPU, radiusMemoryMB, extraCPU, extraMemoryMB, allocatableCPU, allocatableMemoryMB,
		), nil
	}

	return true, fmt.Sprintf(
		"Sufficient resources available for upgrade. Current Radius usage: %.2f CPU, %d MB memory. "+
			"Additional resources needed: %.2f CPU, %d MB memory. "+
			"Available: %.2f CPU, %d MB memory",
		radiusCPU, radiusMemoryMB, extraCPU, extraMemoryMB, allocatableCPU, allocatableMemoryMB,
	), nil
}

// ResourceQuota represents CPU and memory resource quantities.
type ResourceQuota struct {
	CPU    int64 // milliCPU
	Memory int64 // bytes
}

// getClusterResources calculates total cluster capacity and allocatable resources.
func (k *KubernetesResourceCheck) getClusterResources(ctx context.Context, clientset kubernetes.Interface) (ResourceQuota, ResourceQuota, error) {
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return ResourceQuota{}, ResourceQuota{}, fmt.Errorf("failed to list nodes: %w", err)
	}

	var totalCapacity, totalAllocatable ResourceQuota

	for _, node := range nodes.Items {
		// Skip nodes that are not ready or schedulable
		if !k.isNodeReady(node) || !k.isNodeSchedulable(node) {
			continue
		}

		// Add node capacity
		if cpu := node.Status.Capacity.Cpu(); cpu != nil {
			totalCapacity.CPU += cpu.MilliValue()
		}
		if memory := node.Status.Capacity.Memory(); memory != nil {
			totalCapacity.Memory += memory.Value()
		}

		// Add node allocatable
		if cpu := node.Status.Allocatable.Cpu(); cpu != nil {
			totalAllocatable.CPU += cpu.MilliValue()
		}
		if memory := node.Status.Allocatable.Memory(); memory != nil {
			totalAllocatable.Memory += memory.Value()
		}
	}

	return totalCapacity, totalAllocatable, nil
}

// getCurrentRadiusResourceUsage gets the current resource requests of Radius components.
func (k *KubernetesResourceCheck) getCurrentRadiusResourceUsage(ctx context.Context, clientset kubernetes.Interface) (ResourceQuota, error) {
	deployments, err := clientset.AppsV1().Deployments("radius-system").List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/part-of=radius",
	})
	if err != nil {
		return ResourceQuota{}, fmt.Errorf("failed to list Radius deployments: %w", err)
	}

	var totalUsage ResourceQuota

	for _, deployment := range deployments.Items {
		for _, container := range deployment.Spec.Template.Spec.Containers {
			if container.Resources.Requests != nil {
				if cpu := container.Resources.Requests.Cpu(); cpu != nil {
					totalUsage.CPU += cpu.MilliValue()
				}
				if memory := container.Resources.Requests.Memory(); memory != nil {
					totalUsage.Memory += memory.Value()
				}
			}
		}

		// Multiply by replica count
		if deployment.Spec.Replicas != nil {
			totalUsage.CPU *= int64(*deployment.Spec.Replicas)
			totalUsage.Memory *= int64(*deployment.Spec.Replicas)
		}
	}

	return totalUsage, nil
}

// getEstimatedRadiusResourceUsage provides default estimates if we can't get actual usage.
func (k *KubernetesResourceCheck) getEstimatedRadiusResourceUsage() ResourceQuota {
	// Conservative estimates based on typical Radius deployment
	return ResourceQuota{
		CPU:    2000,                   // 2 CPU cores in milliCPU
		Memory: 4 * 1024 * 1024 * 1024, // 4 GB in bytes
	}
}

// isNodeReady checks if a node is in ready state.
func (k *KubernetesResourceCheck) isNodeReady(node corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

// isNodeSchedulable checks if a node is schedulable.
func (k *KubernetesResourceCheck) isNodeSchedulable(node corev1.Node) bool {
	return !node.Spec.Unschedulable
}
