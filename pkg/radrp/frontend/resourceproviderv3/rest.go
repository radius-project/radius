// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resourceproviderv3

import "github.com/Azure/radius/pkg/radrp/rest"

// ApplicationResource represents a Radius application in the ARM wire-format.
type ApplicationResource struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Name       string                 `json:"name"`
	Tags       map[string]string      `json:"tags,omitempty"`
	Location   string                 `json:"omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// RadiusResource represents one of the child resources of Application in the ARM wire-format.
type RadiusResource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`

	// Combination of status, provisioning state and definition,
	Properties map[string]interface{} `json:"properties,omitempty"`
}

type RadiusResourceStatus = rest.ComponentStatus
