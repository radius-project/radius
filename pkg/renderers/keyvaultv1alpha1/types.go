// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package keyvaultv1alpha1

import (
	"github.com/Azure/radius/pkg/azure/azresources"
)

const Kind = "azure.com/KeyVault@v1alpha1"

var KeyVaultResourceType = azresources.KnownType{
	Types: []azresources.ResourceType{
		{
			Type: azresources.KeyVaultVaults,
			Name: "*",
		},
	},
}

// KeyVaultComponent is the definition of the keyvault component
type KeyVaultComponent struct {
	Name     string                   `json:"name"`
	Kind     string                   `json:"kind"`
	Config   KeyVaultConfig           `json:"config,omitempty"`
	Run      map[string]interface{}   `json:"run,omitempty"`
	Uses     []map[string]interface{} `json:"uses,omitempty"`
	Bindings []map[string]interface{} `json:"bindings,omitempty"`
	Traits   []map[string]interface{} `json:"traits,omitempty"`
}

// KeyVaultConfig is the defintion of the config section
type KeyVaultConfig struct {
	Managed  bool   `json:"managed"`
	Resource string `json:"resource"`
}
