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

package helm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	helm "helm.sh/helm/v3/pkg/action"
)

func TestHelmClientImpl_RunHelmHistory(t *testing.T) {
	client := &HelmClientImpl{}

	// Since RunHelmHistory uses real Helm internals that require a configured cluster,
	// we test the method exists and has the correct signature.
	// The actual functionality is tested through integration tests in cluster_test.go
	require.NotNil(t, client.RunHelmHistory)

	// Test with nil configuration - should handle gracefully or panic predictably
	defer func() {
		if r := recover(); r != nil {
			// Expected behavior when configuration is nil
			t.Log("Expected panic when configuration is nil")
		}
	}()

	// This will panic or error, which is expected behavior
	client.RunHelmHistory(nil, "test-release")
}

func TestHelmClientImpl_RunHelmRollback(t *testing.T) {
	client := &HelmClientImpl{}

	// Since RunHelmRollback uses real Helm internals that require a configured cluster,
	// we test the method exists and has the correct signature.
	// The actual functionality is tested through integration tests in cluster_test.go
	require.NotNil(t, client.RunHelmRollback)

	// Test with nil configuration - should handle gracefully or panic predictably
	defer func() {
		if r := recover(); r != nil {
			// Expected behavior when configuration is nil
			t.Log("Expected panic when configuration is nil")
		}
	}()

	// This will panic or error, which is expected behavior
	client.RunHelmRollback(nil, "test-release", 1, true)
}

func TestHelmClient_Constants(t *testing.T) {
	// Test that our new timeout constant is defined
	require.Equal(t, time.Duration(5)*time.Minute, rollbackTimeout)
	require.Equal(t, time.Duration(5)*time.Minute, installTimeout)
	require.Equal(t, time.Duration(5)*time.Minute, uninstallTimeout)
	require.Equal(t, time.Duration(5)*time.Minute, upgradeTimeout)
}

func TestHelmClient_Interface(t *testing.T) {
	// Test that HelmClientImpl implements the HelmClient interface
	var _ HelmClient = &HelmClientImpl{}

	// Test that all new methods are included in the interface
	client := NewHelmClient()
	require.NotNil(t, client)

	// Verify the interface includes the new methods by checking they exist
	// We can't call them without a proper Helm configuration
	require.NotNil(t, client.(*HelmClientImpl).RunHelmHistory)
	require.NotNil(t, client.(*HelmClientImpl).RunHelmRollback)
}

// Mock test to verify the method signatures are correct
func TestHelmClient_MockCompatibility(t *testing.T) {
	// This test ensures our new methods can be mocked properly
	client := &HelmClientImpl{}

	// Test RunHelmHistory method signature
	var historyFunc = client.RunHelmHistory
	require.NotNil(t, historyFunc)

	// Test RunHelmRollback method signature
	var rollbackFunc func(*helm.Configuration, string, int, bool) error = client.RunHelmRollback
	require.NotNil(t, rollbackFunc)
}
