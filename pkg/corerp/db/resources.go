// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package db

import "github.com/project-radius/radius/pkg/resourcemodel"

// RadiusResource represents one of the child resources of Application as stored in the database.
type RadiusResource struct {
	ID             string                 `json:"id"`
	Definition     map[string]interface{} `json:"definition"`
	ComputedValues map[string]interface{} `json:"computedValues"`

	// NOTE: this is not part of the output of the RP - this is internal tracking
	// for how we can look up values that do not store.
	SecretValues map[string]SecretValueReference `json:"secretValues"`

	Status            RadiusResourceStatus `json:"status"`
	ProvisioningState string               `json:"provisioningState"`
}

type RadiusResourceStatus struct {
	ProvisioningState string           `json:"provisioningState"`
	OutputResources   []OutputResource `json:"outputResources,omitempty" structs:"-"` // Ignore stateful property during serialization
}

// see renderers.SecretValueReference for description
type SecretValueReference struct {
	LocalID       string                     `json:"localId"`
	Action        string                     `json:"action,omitempty"`
	ValueSelector string                     `json:"valueSelector"`
	Transformer   resourcemodel.ResourceType `json:"transformer,omitempty"`
	Value         *string                    `json:"value,omitempty"`
}
