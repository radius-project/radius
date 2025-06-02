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
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestKubernetesResourceCheck_Properties(t *testing.T) {
	check := NewKubernetesResourceCheck("test-context")
	assert.Equal(t, "Kubernetes Resource Availability", check.Name())
	assert.Equal(t, SeverityWarning, check.Severity())
}

func TestKubernetesResourceCheck_IsNodeReady(t *testing.T) {
	check := NewKubernetesResourceCheck("test-context")

	tests := []struct {
		name     string
		node     corev1.Node
		expected bool
	}{
		{
			name: "ready node",
			node: corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "not ready node",
			node: corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "no ready condition",
			node: corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := check.isNodeReady(tt.node)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestKubernetesResourceCheck_IsNodeSchedulable(t *testing.T) {
	check := NewKubernetesResourceCheck("test-context")

	tests := []struct {
		name     string
		node     corev1.Node
		expected bool
	}{
		{
			name: "schedulable node",
			node: corev1.Node{
				Spec: corev1.NodeSpec{
					Unschedulable: false,
				},
			},
			expected: true,
		},
		{
			name: "unschedulable node",
			node: corev1.Node{
				Spec: corev1.NodeSpec{
					Unschedulable: true,
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := check.isNodeSchedulable(tt.node)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestKubernetesResourceCheck_GetEstimatedRadiusResourceUsage(t *testing.T) {
	check := NewKubernetesResourceCheck("test-context")

	usage := check.getEstimatedRadiusResourceUsage()

	// Verify reasonable defaults
	assert.Equal(t, int64(2000), usage.CPU)                // 2 CPU cores in milliCPU
	assert.Equal(t, int64(4*1024*1024*1024), usage.Memory) // 4 GB in bytes
}
