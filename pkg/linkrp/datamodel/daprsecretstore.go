// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/rp"
)

type DaprSecretStoreKind string

const (
	DaprSecretStoreKindGeneric DaprSecretStoreKind = "generic"
	DaprSecretStoreKindUnknown DaprSecretStoreKind = "unknown"
)

type DaprSecretStorePropertiesMode string

const (
	DaprSecretStorePropertiesModeRecipe  DaprSecretStorePropertiesMode = "recipe"
	DaprSecretStorePropertiesModeValues  DaprSecretStorePropertiesMode = "values"
	DaprSecretStorePropertiesModeUnknown DaprSecretStorePropertiesMode = "unknown"
)

// DaprSecretStore represents DaprSecretStore link resource.
type DaprSecretStore struct {
	v1.TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData v1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties DaprSecretStoreProperties `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	v1.InternalMetadata

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

func (daprSecretStore DaprSecretStore) ResourceTypeName() string {
	return "Applications.Link/daprSecretStores"
}

// DaprSecretStoreProperties represents the properties of DaprSecretStore resource.
type DaprSecretStoreProperties struct {
	rp.BasicResourceProperties
	rp.BasicDaprResourceProperties
	ProvisioningState v1.ProvisioningState          `json:"provisioningState,omitempty"`
	Mode              DaprSecretStorePropertiesMode `json:"mode"`
	Kind              DaprSecretStoreKind           `json:"kind"`
	Type              string                        `json:"type"`
	Version           string                        `json:"version"`
	Metadata          map[string]interface{}        `json:"metadata"`
	Recipe            LinkRecipe                    `json:"recipe,omitempty"`
}
