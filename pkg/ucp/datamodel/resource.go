// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

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
