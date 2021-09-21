// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resourceproviderv3

import "github.com/Azure/radius/pkg/radrp/rest"

// ApplicationResource represents a Radius Application in the ARM wire-format.
type ApplicationResource struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Name       string                 `json:"name"`
	Tags       map[string]string      `json:"tags,omitempty"`
	Location   string                 `json:"omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// ApplicationResource represents a list of Radius Applications in the ARM wire-format.
type ApplicationResourceList struct {
	Value []ApplicationResource `json:"value"`
}

// RadiusResource represents one of the child resource types of Application in the ARM wire-format.
type RadiusResource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`

	// Combination of status, provisioning state and definition,
	Properties map[string]interface{} `json:"properties,omitempty"`
}

type RadiusResourceStatus = rest.ComponentStatus

// RadiusResourceList represents a list of a child resource type of Application in the ARM wire-format.
type RadiusResourceList struct {
	Value []RadiusResource `json:"value"`
}
