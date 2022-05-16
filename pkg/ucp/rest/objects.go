// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rest

type PlaneProperties struct {
	ResourceProviders map[string]string `json:"resourceProviders"`
	Kind              string            `json:"kind"`
}
type Plane struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Name       string          `json:"name"`
	Properties PlaneProperties `json:"properties"`
}

//PlaneList represents a list of UCP planes in the ARM wire-format
type PlaneList struct {
	Value []Plane `json:"value"`
}

// ResourceGroup represents a resource group within UCP
type ResourceGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ResourceGroupList represents a list of resource groups
type ResourceGroupList struct {
	Value []ResourceGroup `json:"value"`
}
