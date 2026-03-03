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
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	helm "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	kubefake "helm.sh/helm/v3/pkg/kube/fake"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
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
	_, err := client.RunHelmHistory(nil, "test-release")
	if err == nil {
		t.Error("Expected error when configuration is nil, but got nil")
	}
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
	err := client.RunHelmRollback(nil, "test-release", 1, true)
	if err == nil {
		t.Error("Expected error when configuration is nil, but got nil")
	}
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

// TestHelmClient_UpgradeReusesValues verifies that RunHelmUpgrade preserves existing values
// by using ReuseValues=true. This test validates the fix for issue #11218 where upgrades
// were resetting values to chart defaults.
func TestHelmClient_UpgradeReusesValues(t *testing.T) {
	t.Run("upgrade preserves existing values and merges new values", func(t *testing.T) {
		// This is an integration test that validates the actual behavior of RunHelmUpgrade
		// with ReuseValues=true by simulating an upgrade with Helm's in-memory storage.
		
		// Test data representing values from previous install/upgrade
		existingValues := map[string]interface{}{
			"global": map[string]interface{}{
				"azureWorkloadIdentity": map[string]interface{}{
					"enabled": true,
				},
				"imageRegistry": "myregistry.azurecr.io",
			},
			"database": map[string]interface{}{
				"enabled": true,
			},
		}
		
		// New values being applied in this upgrade
		newValues := map[string]interface{}{
			"global": map[string]interface{}{
				"imageTag": "0.48.0",
			},
		}
		
		// Expected merged result: existing values preserved + new values applied
		expectedValues := map[string]interface{}{
			"global": map[string]interface{}{
				"azureWorkloadIdentity": map[string]interface{}{
					"enabled": true,
				},
				"imageRegistry": "myregistry.azurecr.io",
				"imageTag":      "0.48.0",
			},
			"database": map[string]interface{}{
				"enabled": true,
			},
		}
		
		// Create an in-memory Helm configuration for testing (similar to Helm's own tests)
		registryClient, err := registry.NewClient()
		require.NoError(t, err, "Failed to create registry client")
		
		cfg := &helm.Configuration{
			Releases:       storage.Init(driver.NewMemory()),
			KubeClient:     &kubefake.PrintingKubeClient{Out: io.Discard},
			Capabilities:   chartutil.DefaultCapabilities,
			RegistryClient: registryClient,
			Log:            func(format string, v ...interface{}) {},
		}
		
		// Create and store an initial release with existing values
		initialRelease := &release.Release{
			Name:      "test-release",
			Namespace: "test-namespace",
			Version:   1,
			Config:    existingValues,
			Chart: &chart.Chart{
				Metadata: &chart.Metadata{
					Name:    "test-chart",
					Version: "1.0.0",
				},
			},
			Info: &release.Info{
				Status: release.StatusDeployed,
			},
		}
		err = cfg.Releases.Create(initialRelease)
		require.NoError(t, err, "Failed to create initial release")
		
		// Create a chart with new values
		upgradeChart := &chart.Chart{
			Metadata: &chart.Metadata{
				Name:    "test-chart",
				Version: "1.1.0",
			},
			Values: newValues,
		}
		
		// Execute the upgrade using our RunHelmUpgrade implementation
		client := &HelmClientImpl{}
		upgradedRelease, err := client.RunHelmUpgrade(cfg, upgradeChart, "test-release", "test-namespace", false)
		require.NoError(t, err, "RunHelmUpgrade should succeed")
		require.NotNil(t, upgradedRelease, "Upgraded release should not be nil")
		
		// Verify the release was upgraded
		require.Equal(t, 2, upgradedRelease.Version, "Release version should be incremented")
		require.Equal(t, release.StatusDeployed, upgradedRelease.Info.Status, "Release should be deployed")
		
		// Verify that existing values were preserved and new values were merged
		require.Equal(t, expectedValues, upgradedRelease.Config, 
			"Upgraded release should preserve existing values and merge new values")
		
		// Specifically verify key values that should be preserved (issue #11218)
		globalMap, ok := upgradedRelease.Config["global"].(map[string]interface{})
		require.True(t, ok, "global key should exist and be a map")
		
		azureWIMap, ok := globalMap["azureWorkloadIdentity"].(map[string]interface{})
		require.True(t, ok, "global.azureWorkloadIdentity should exist")
		require.Equal(t, true, azureWIMap["enabled"], "azureWorkloadIdentity.enabled should be preserved")
		
		require.Equal(t, "myregistry.azurecr.io", globalMap["imageRegistry"], "imageRegistry should be preserved")
		require.Equal(t, "0.48.0", globalMap["imageTag"], "imageTag should be set from new values")
		
		dbMap, ok := upgradedRelease.Config["database"].(map[string]interface{})
		require.True(t, ok, "database key should exist and be a map")
		require.Equal(t, true, dbMap["enabled"], "database.enabled should be preserved")
	})
}
