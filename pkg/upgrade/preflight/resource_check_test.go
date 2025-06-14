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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestKubernetesResourceCheck_Properties(t *testing.T) {
	check := NewKubernetesResourceCheck("test-context")
	assert.Equal(t, "Kubernetes Resource Availability", check.Name())
	assert.Equal(t, SeverityWarning, check.Severity())
}

func TestKubernetesResourceCheck_WithClientset(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	check := NewKubernetesResourceCheckWithClientset("test-context", clientset)

	assert.Equal(t, "test-context", check.kubeContext)
	assert.NotNil(t, check.clientset)
}

func TestKubernetesResourceCheck_Run(t *testing.T) {
	ctx := context.Background()

	t.Run("sufficient resources", func(t *testing.T) {
		clientset := createClientsetWithResources(
			createNode("node1", "4000m", "8Gi"), // 4 CPU, 8GB
			createRadiusDeployment("ucp", "500m", "1Gi", 1),
		)

		check := NewKubernetesResourceCheckWithClientset("test", clientset)
		pass, msg, err := check.Run(ctx)

		require.NoError(t, err)
		assert.True(t, pass)
		assert.Contains(t, msg, "Sufficient resources available")
	})

	t.Run("insufficient resources", func(t *testing.T) {
		clientset := createClientsetWithResources(
			createNode("node1", "100m", "50Mi"),              // Very limited: 0.1 CPU, 50MB
			createRadiusDeployment("ucp", "1000m", "1Gi", 1), // Needs 1 CPU, 1GB, extra 0.5 CPU, 0.5GB for upgrade
		)
		// Extra needs: 500m CPU, 512MB memory
		// Available: 100m CPU, 50MB memory - clearly insufficient

		check := NewKubernetesResourceCheckWithClientset("test", clientset)
		pass, msg, err := check.Run(ctx)

		require.NoError(t, err)
		assert.False(t, pass)
		assert.Contains(t, msg, "Insufficient resources")
	})

	t.Run("no radius deployments found", func(t *testing.T) {
		clientset := createClientsetWithResources(
			createNode("node1", "4000m", "8Gi"),
		)

		check := NewKubernetesResourceCheckWithClientset("test", clientset)
		pass, _, err := check.Run(ctx)

		require.NoError(t, err)
		// Should still pass using estimated usage
		assert.True(t, pass)
	})

	t.Run("no nodes found - uses estimated usage", func(t *testing.T) {
		// Create a scenario with no nodes but namespace exists (so estimated usage is used)
		clientset := fake.NewSimpleClientset()
		// Simulate error when getting radius deployments to force estimated usage
		clientset.PrependReactor("list", "deployments", func(action clienttesting.Action) (bool, runtime.Object, error) {
			return true, nil, fmt.Errorf("namespace not found")
		})

		check := NewKubernetesResourceCheckWithClientset("test", clientset)
		pass, msg, err := check.Run(ctx)

		require.NoError(t, err)
		// With no nodes (allocatable=0) but estimated usage > 0, should fail
		assert.False(t, pass)
		assert.Contains(t, msg, "Insufficient resources")
	})

	t.Run("connection failure", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		clientset.PrependReactor("*", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
			return true, nil, fmt.Errorf("connection refused")
		})

		check := NewKubernetesResourceCheckWithClientset("test", clientset)
		pass, _, err := check.Run(ctx)

		require.Error(t, err)
		assert.False(t, pass)
		assert.Contains(t, err.Error(), "connection refused")
	})
}

func TestKubernetesResourceCheck_NodeHelpers(t *testing.T) {
	check := NewKubernetesResourceCheck("test-context")

	t.Run("isNodeReady", func(t *testing.T) {
		readyNode := corev1.Node{
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		}
		assert.True(t, check.isNodeReady(readyNode))

		notReadyNode := corev1.Node{
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
				},
			},
		}
		assert.False(t, check.isNodeReady(notReadyNode))
	})

	t.Run("isNodeSchedulable", func(t *testing.T) {
		schedulableNode := corev1.Node{
			Spec: corev1.NodeSpec{Unschedulable: false},
		}
		assert.True(t, check.isNodeSchedulable(schedulableNode))

		unschedulableNode := corev1.Node{
			Spec: corev1.NodeSpec{Unschedulable: true},
		}
		assert.False(t, check.isNodeSchedulable(unschedulableNode))
	})
}

func TestKubernetesResourceCheck_EstimatedUsage(t *testing.T) {
	check := NewKubernetesResourceCheck("test-context")
	usage := check.getEstimatedRadiusResourceUsage()

	assert.Equal(t, int64(2000), usage.CPU)                // 2 CPU cores in milliCPU
	assert.Equal(t, int64(4*1024*1024*1024), usage.Memory) // 4 GB in bytes
}

func TestKubernetesResourceCheck_NoClientset(t *testing.T) {
	check := NewKubernetesResourceCheck("nonexistent-context")

	pass, _, err := check.Run(context.Background())

	require.Error(t, err)
	assert.False(t, pass)
	assert.Contains(t, err.Error(), "failed to create Kubernetes client")
}

// Helper functions for creating test objects

func createNode(name, cpu, memory string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(cpu),
				corev1.ResourceMemory: resource.MustParse(memory),
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(cpu),
				corev1.ResourceMemory: resource.MustParse(memory),
			},
		},
		Spec: corev1.NodeSpec{Unschedulable: false},
	}
}

func createRadiusDeployment(name, cpu, memory string, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "radius-system",
			Labels:    map[string]string{"app.kubernetes.io/part-of": "radius"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(cpu),
									corev1.ResourceMemory: resource.MustParse(memory),
								},
							},
						},
					},
				},
			},
		},
	}
}

func createClientsetWithResources(objects ...runtime.Object) *fake.Clientset {
	return fake.NewSimpleClientset(objects...)
}
