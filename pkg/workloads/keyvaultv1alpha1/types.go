// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package keyvaultv1alpha1

import "github.com/Azure/azure-sdk-for-go/profiles/latest/keyvault/mgmt/keyvault"

// KeyVaultComponent is the definition of the keyvault component
type KeyVaultComponent struct {
	Name      string                   `json:"name"`
	Kind      string                   `json:"kind"`
	Config    KeyVaultConfig           `json:"config,omitempty"`
	Run       map[string]interface{}   `json:"run,omitempty"`
	DependsOn []map[string]interface{} `json:"dependson,omitempty"`
	Provides  []map[string]interface{} `json:"provides,omitempty"`
	Traits    []map[string]interface{} `json:"traits,omitempty"`
}

// KeyVaultConfig is the defintion of the config section
type KeyVaultConfig struct {
	Managed                bool                              `json:"managed"`
	KeyPermissions         []keyvault.KeyPermissions         `json:"keypermissions"`
	SecretPermissions      []keyvault.SecretPermissions      `json:"secretpermissions"`
	CertificatePermissions []keyvault.CertificatePermissions `json:"certificatepermissions"`
}
