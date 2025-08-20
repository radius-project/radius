package terraform

import (
    "encoding/json"
    "os"
    "path/filepath"
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
