// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rest

type PlaneProperties struct {
	ResourceProviders map[string]string `json:"resourceProviders" yaml:"resourceProviders"`
	Kind              string            `json:"kind" yaml:"kind"`
}
type Plane struct {
	ID         string          `json:"id" yaml:"id"`
	Type       string          `json:"type" yaml:"type"`
	Name       string          `json:"name" yaml:"name"`
	Properties PlaneProperties `json:"properties" yaml:"properties"`
}

//PlaneList represents a list of UCP planes in the ARM wire-format
type PlaneList struct {
	Value []Plane `json:"value" yaml:"value"`
}

// ResourceGroup represents a resource group within UCP
type ResourceGroup struct {
	ID   string `json:"id" yaml:"id"`
	Name string `json:"name" yaml:"name"`
}

// ResourceGroupList represents a list of resource groups
type ResourceGroupList struct {
	Value []ResourceGroup `json:"value" yaml:"value"`
}
