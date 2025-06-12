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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestKubernetesConnectivityCheck_Properties(t *testing.T) {
	check := NewKubernetesConnectivityCheck("test-context")
	assert.Equal(t, "Kubernetes Connectivity", check.Name())
	assert.Equal(t, SeverityError, check.Severity())
}

func TestKubernetesConnectivityCheck_WithClientset(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	check := NewKubernetesConnectivityCheckWithClientset("test-context", clientset)

	assert.Equal(t, "test-context", check.kubeContext)
	assert.NotNil(t, check.clientset)
}

func TestKubernetesConnectivityCheck_Run(t *testing.T) {
	ctx := context.Background()

	t.Run("successful connection with permissions", func(t *testing.T) {
		// Setup: namespace exists, deployments can be listed
		clientset := fake.NewSimpleClientset(
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: RadiusSystemNamespace},
			},
		)

		check := NewKubernetesConnectivityCheckWithClientset("test", clientset)
		pass, msg, err := check.Run(ctx)

		require.NoError(t, err)
		assert.True(t, pass)
		assert.Contains(t, msg, "with sufficient permissions")
	})

	t.Run("namespace not found", func(t *testing.T) {
		// Setup: empty cluster
		clientset := fake.NewSimpleClientset()

		check := NewKubernetesConnectivityCheckWithClientset("test", clientset)
		pass, msg, err := check.Run(ctx)

		require.NoError(t, err)
		assert.False(t, pass) // This is a failure for upgrade preflight
		assert.Contains(t, msg, "namespace not found")
	})

	t.Run("insufficient permissions", func(t *testing.T) {
		// Setup: namespace exists but can't list deployments
		clientset := fake.NewSimpleClientset(
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: RadiusSystemNamespace},
			},
		)

		// Simulate permission error
		clientset.PrependReactor("list", "deployments", func(action clienttesting.Action) (bool, runtime.Object, error) {
			return true, nil, fmt.Errorf("forbidden")
		})

		check := NewKubernetesConnectivityCheckWithClientset("test", clientset)
		pass, msg, err := check.Run(ctx)

		require.NoError(t, err)
		assert.False(t, pass)
		assert.Contains(t, msg, "insufficient permissions")
	})

	t.Run("connection failure", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()

		// Simulate connection error on discovery calls
		clientset.PrependReactor("*", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
			return true, nil, fmt.Errorf("connection refused")
		})

		check := NewKubernetesConnectivityCheckWithClientset("test", clientset)
		pass, msg, err := check.Run(ctx)

		require.Error(t, err)
		assert.False(t, pass)
		assert.Equal(t, "Cannot connect to Kubernetes cluster", msg)
		assert.Contains(t, err.Error(), "connection refused")
	})
}

func TestKubernetesConnectivityCheck_NoClientset(t *testing.T) {
	// This tests the scenario where no clientset is provided
	// In a test environment, this should fail to create a client
	check := NewKubernetesConnectivityCheck("nonexistent-context")

	pass, _, err := check.Run(context.Background())

	require.Error(t, err)
	assert.False(t, pass)
	assert.Contains(t, err.Error(), "failed to create Kubernetes client")
}
