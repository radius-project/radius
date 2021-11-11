// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resourceprovider

import "github.com/Azure/radius/pkg/radrp/rest"

// ApplicationResource represents a Radius Application in the ARM wire-format.
type ApplicationResource struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Name       string                 `json:"name"`
	Tags       map[string]string      `json:"tags,omitempty"`
	Location   string                 `json:"location,omitempty"`
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

// AzureResource over the wire format for non-Radius Azure resources that are referenced from Radius resources in the application. These resources do not have output resources and may not support all the other properties included in RadiusResource type.
type AzureResource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
	Type string `json:"type"`
}

// ListSecretsInput is used for the RP's 'listSecrets' custom action.
type ListSecretsInput struct {
	// TargetID is the resource ID of the Radius resource for which secrets are being listed.
	TargetID string `json:"targetId"`
}
