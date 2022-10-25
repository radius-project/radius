// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

// Resource represents a resource within a UCP resource group
type Resource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// ResourceList represents a list of resources
type ResourceList struct {
	Value []Resource `json:"value" yaml:"value"`
}
