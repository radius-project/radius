// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestorev1alpha1

import (
	"github.com/Azure/radius/pkg/azure/azresources"
)

const Kind = "dapr.io/StateStore@v1alpha1"

var StorageAccountResourceType = azresources.KnownType{
	Types: []azresources.ResourceType{
		{
			Type: azresources.StorageStorageAccounts,
			Name: "*",
		},
	},
}

// DaprStateStoreComponent is the definition of the container component
type DaprStateStoreComponent struct {
	Name     string                   `json:"name"`
	Kind     string                   `json:"kind"`
	Config   DaprStateStoreConfig     `json:"config,omitempty"`
	Run      map[string]interface{}   `json:"run,omitempty"`
	Uses     []map[string]interface{} `json:"uses,omitempty"`
	Bindings []map[string]interface{} `json:"bindings,omitempty"`
	Traits   []map[string]interface{} `json:"traits,omitempty"`
}

// DaprStateStoreConfig is the defintion of the config section
type DaprStateStoreConfig struct {
	Kind     string `json:"kind"`
	Managed  bool   `json:"managed"`
	Resource string `json:"resource"`
}
