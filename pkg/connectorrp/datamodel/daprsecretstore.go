// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
)

type DaprSecretStoreKind string

const (
	DaprSecretStoreKindGeneric DaprSecretStoreKind = "generic"
	DaprSecretStoreKindUnknown DaprSecretStoreKind = "unknown"
)

// DaprSecretStore represents DaprSecretStore connector resource.
type DaprSecretStore struct {
	v1.TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData v1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties DaprSecretStoreProperties `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	v1.InternalMetadata
}

func (daprSecretStore DaprSecretStore) ResourceTypeName() string {
	return "Applications.Connector/daprSecretStores"
}

// DaprSecretStoreProperties represents the properties of DaprSecretStore resource.
type DaprSecretStoreProperties struct {
	v1.BasicResourceProperties
	ProvisioningState v1.ProvisioningState   `json:"provisioningState,omitempty"`
	Environment       string                 `json:"environment"`
	Application       string                 `json:"application,omitempty"`
	Kind              DaprSecretStoreKind    `json:"kind"`
	Type              string                 `json:"type"`
	Version           string                 `json:"version"`
	Metadata          map[string]interface{} `json:"metadata"`
	SecretStoreName    string                 `json:"secretStoreName"`
}
