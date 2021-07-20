// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package keyvaultv1alpha1

import (
	"github.com/Azure/radius/pkg/azresources"
	"github.com/Azure/radius/pkg/radrp/resources"
)

const Kind = "azure.com/KeyVault@v1alpha1"

var KeyVaultResourceType = resources.KnownType{
	Types: []resources.ResourceType{
		{
			Type: azresources.KeyVaultVaults,
			Name: "*",
		},
	},
}

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
	Managed  bool   `json:"managed"`
	Resource string `json:"resource"`
}
