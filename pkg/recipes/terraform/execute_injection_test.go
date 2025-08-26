package terraform

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_injectIntoHCLFile_InsertsAndOverrides(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.tf")
	// Existing provider block with a features block and a placeholder tenant_id
	original := `
provider "azurerm" {
  features {}
  tenant_id = "old-tenant"
}
`
	require.NoError(t, os.WriteFile(file, []byte(original), 0644))

	e := &executor{}
	cfg := map[string]map[string]any{
		"azurerm": {
			"client_id":            "my-client-id",
			"tenant_id":            "new-tenant",
			"use_oidc":             true,
			"use_cli":              false,
			"oidc_token_file_path": "/var/run/secrets/azure/tokens/azure-identity-token",
		},
	}

	err := e.injectIntoHCLFile(t.Context(), file, cfg)
	require.NoError(t, err)

	data, err := os.ReadFile(file)
	require.NoError(t, err)
	s := string(data)

	// Expect overrides and additions
	require.Contains(t, s, "tenant_id = \"new-tenant\"")
	require.Contains(t, s, "client_id = \"my-client-id\"")
	require.Contains(t, s, "use_oidc = true")
	require.Contains(t, s, "use_cli = false")
	require.Contains(t, s, "oidc_token_file_path = \"/var/run/secrets/azure/tokens/azure-identity-token\"")
	// Preserve features block
	require.Contains(t, s, "features {}")
}

func Test_injectIntoJSONFile_MergesConfig(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.tf.json")
	obj := map[string]any{
		"provider": map[string]any{
			"azurerm": []any{
				map[string]any{},
			},
		},
	}
	raw, _ := json.MarshalIndent(obj, "", "  ")
	require.NoError(t, os.WriteFile(file, raw, 0644))

	e := &executor{}
	cfg := map[string]map[string]any{
		"azurerm": {
			"subscription_id":      "sub-id",
			"client_id":            "client",
			"tenant_id":            "tenant",
			"use_oidc":             true,
			"oidc_token_file_path": "/var/run/secrets/azure/tokens/azure-identity-token",
		},
	}

	err := e.injectIntoJSONFile(t.Context(), file, cfg)
	require.NoError(t, err)

	data, err := os.ReadFile(file)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(data, &out))

	prov := out["provider"].(map[string]any)
	arr := prov["azurerm"].([]any)
	got := arr[0].(map[string]any)
	require.Equal(t, "sub-id", got["subscription_id"])
	require.Equal(t, "client", got["client_id"])
	require.Equal(t, "tenant", got["tenant_id"])
	require.Equal(t, true, got["use_oidc"])
	require.Equal(t, "/var/run/secrets/azure/tokens/azure-identity-token", got["oidc_token_file_path"])
}

func Test_injectIntoHCLFile_AzurermWithComplexFeatures(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.tf")
	// Provider block with complex nested features - like the user's example
	original := `
provider "azurerm" {
  features {
    resource_group {
      prevent_deletion_if_contains_resources = false
    }
  }
}
`
	require.NoError(t, os.WriteFile(file, []byte(original), 0644))

	e := &executor{}
	cfg := map[string]map[string]any{
		"azurerm": {
			"subscription_id":      "66d1209e-1382-45d3-99bb-650e6bf63fc0",
			"client_id":            "1638c7ef-6baf-4c39-a985-e6b3733d7df5",
			"tenant_id":            "e32125e8-f6f6-4ff0-b464-b0baf30a4f00",
			"use_oidc":             true,
			"use_cli":              false,
			"oidc_token_file_path": "/var/run/secrets/azure/tokens/azure-identity-token",
		},
	}

	err := e.injectIntoHCLFile(t.Context(), file, cfg)
	require.NoError(t, err)

	data, err := os.ReadFile(file)
	require.NoError(t, err)
	s := string(data)

	// Verify Azure credentials were injected
	require.Contains(t, s, "subscription_id = \"66d1209e-1382-45d3-99bb-650e6bf63fc0\"")
	require.Contains(t, s, "client_id = \"1638c7ef-6baf-4c39-a985-e6b3733d7df5\"")
	require.Contains(t, s, "tenant_id = \"e32125e8-f6f6-4ff0-b464-b0baf30a4f00\"")
	require.Contains(t, s, "use_oidc = true")
	require.Contains(t, s, "use_cli = false")
	require.Contains(t, s, "oidc_token_file_path = \"/var/run/secrets/azure/tokens/azure-identity-token\"")

	// Verify complex nested features block is preserved
	require.Contains(t, s, "features {")
	require.Contains(t, s, "resource_group {")
	require.Contains(t, s, "prevent_deletion_if_contains_resources = false")

	// Verify no extra closing braces
	require.NotContains(t, s, "}}")
}

func Test_injectIntoHCLFile_DoesNotInjectIntoOtherProviders(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.tf")
	// Multiple provider blocks - azurerm and datadog
	original := `
provider "azurerm" {
  features {}
}

provider "datadog" {
  api_key = "existing-key"
}

provider "aws" {
  region = "us-west-2"
}
`
	require.NoError(t, os.WriteFile(file, []byte(original), 0644))

	e := &executor{}
	cfg := map[string]map[string]any{
		"azurerm": {
			"subscription_id": "test-sub-id",
			"client_id":       "test-client-id",
			"tenant_id":       "test-tenant-id",
		},
	}

	err := e.injectIntoHCLFile(t.Context(), file, cfg)
	require.NoError(t, err)

	data, err := os.ReadFile(file)
	require.NoError(t, err)
	s := string(data)

	// Verify Azure credentials were injected into azurerm block
	require.Contains(t, s, "subscription_id = \"test-sub-id\"")
	require.Contains(t, s, "client_id = \"test-client-id\"")
	require.Contains(t, s, "tenant_id = \"test-tenant-id\"")

	// Verify datadog provider block was NOT modified
	require.Contains(t, s, "api_key = \"existing-key\"")

	// Verify AWS provider block was NOT modified
	require.Contains(t, s, "region = \"us-west-2\"")

	// Verify no Azure credentials leaked into other provider blocks by parsing line by line
	lines := strings.Split(s, "\n")
	inDatadogBlock := false
	inAWSBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(line, "provider \"datadog\"") {
			inDatadogBlock = true
			inAWSBlock = false
			continue
		}
		if strings.Contains(line, "provider \"aws\"") {
			inAWSBlock = true
			inDatadogBlock = false
			continue
		}
		if strings.Contains(line, "provider \"azurerm\"") {
			inDatadogBlock = false
			inAWSBlock = false
			continue
		}
		if trimmed == "}" && (inDatadogBlock || inAWSBlock) {
			// End of provider block
			inDatadogBlock = false
			inAWSBlock = false
			continue
		}
		if inDatadogBlock && (strings.Contains(line, "subscription_id") || strings.Contains(line, "client_id") || strings.Contains(line, "tenant_id")) {
			t.Errorf("Azure credentials leaked into datadog provider block: %s", line)
		}
		if inAWSBlock && (strings.Contains(line, "subscription_id") || strings.Contains(line, "client_id") || strings.Contains(line, "tenant_id")) {
			t.Errorf("Azure credentials leaked into AWS provider block: %s", line)
		}
	}
}

func Test_injectIntoJSONFile_DoesNotInjectIntoOtherProviders(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.tf.json")
	obj := map[string]any{
		"provider": map[string]any{
			"azurerm": []any{
				map[string]any{
					"features": map[string]any{},
				},
			},
			"datadog": []any{
				map[string]any{
					"api_key": "existing-datadog-key",
				},
			},
			"aws": []any{
				map[string]any{
					"region": "us-east-1",
				},
			},
		},
	}
	raw, _ := json.MarshalIndent(obj, "", "  ")
	require.NoError(t, os.WriteFile(file, raw, 0644))

	e := &executor{}
	cfg := map[string]map[string]any{
		"azurerm": {
			"subscription_id": "test-sub-id",
			"client_id":       "test-client-id",
			"tenant_id":       "test-tenant-id",
		},
	}

	err := e.injectIntoJSONFile(t.Context(), file, cfg)
	require.NoError(t, err)

	data, err := os.ReadFile(file)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(data, &out))

	prov := out["provider"].(map[string]any)
	
	// Verify azurerm provider has Azure credentials
	azurermArr := prov["azurerm"].([]any)
	azurermCfg := azurermArr[0].(map[string]any)
	require.Equal(t, "test-sub-id", azurermCfg["subscription_id"])
	require.Equal(t, "test-client-id", azurermCfg["client_id"])
	require.Equal(t, "test-tenant-id", azurermCfg["tenant_id"])

	// Verify datadog provider was NOT modified
	datadogArr := prov["datadog"].([]any)
	datadogCfg := datadogArr[0].(map[string]any)
	require.Equal(t, "existing-datadog-key", datadogCfg["api_key"])
	require.NotContains(t, datadogCfg, "subscription_id")
	require.NotContains(t, datadogCfg, "client_id")
	require.NotContains(t, datadogCfg, "tenant_id")

	// Verify AWS provider was NOT modified
	awsArr := prov["aws"].([]any)
	awsCfg := awsArr[0].(map[string]any)
	require.Equal(t, "us-east-1", awsCfg["region"])
	require.NotContains(t, awsCfg, "subscription_id")
	require.NotContains(t, awsCfg, "client_id")
	require.NotContains(t, awsCfg, "tenant_id")
}
