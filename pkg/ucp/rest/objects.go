// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rest

import "strings"

type PlaneProperties struct {
	ResourceProviders map[string]string `json:"resourceProviders" yaml:"resourceProviders"` // Used only for UCP native planes
	Kind              string            `json:"kind" yaml:"kind"`
	URL               string            `json:"url" yaml:"url"` // Used only for non UCP native planes and non AWS planes
}

// Plane kinds
const (
	PlaneKindUCPNative = "UCPNative"
	PlaneKindAzure     = "Azure"
	PlaneKindAWS       = "AWS"
)

type Plane struct {
	ID         string          `json:"id" yaml:"id"`
	Type       string          `json:"type" yaml:"type"`
	Name       string          `json:"name" yaml:"name"`
	Properties PlaneProperties `json:"properties" yaml:"properties"`
}

// PlaneList represents a list of UCP planes in the ARM wire-format
type PlaneList struct {
	Value []Plane `json:"value" yaml:"value"`
}

// Resource represents a resource within a UCP resource group
type Resource struct {
	ID                string `json:"id" yaml:"id"`
	Name              string `json:"name" yaml:"name"`
	ProvisioningState string `json:"provisioningState" yaml:"provisioningState"`
	Type              string `json:"type" yaml:"type"`
}

// ResourceList represents a list of resources
type ResourceList struct {
	Value []Resource `json:"value" yaml:"value"`
}

func (plane *Plane) LookupResourceProvider(key string) string {
	var value string
	for k, v := range plane.Properties.ResourceProviders {
		if strings.EqualFold(k, key) {
			value = v
			break
		}
	}
	return value
}
