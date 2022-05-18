// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	"github.com/project-radius/radius/pkg/api/armrpcv1"
	"github.com/project-radius/radius/pkg/basedatamodel"
)

type DaprSecretStoreKind string

const (
	DaprSecretStoreKindGeneric DaprSecretStoreKind = "generic"
	DaprSecretStoreKindUnknown DaprSecretStoreKind = "unknown"
)

// DaprSecretStore represents DaprSecretStore connector resource.
type DaprSecretStore struct {
	basedatamodel.TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData armrpcv1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties DaprSecretStoreProperties `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	basedatamodel.InternalMetadata
}

func (daprSecretStore DaprSecretStore) ResourceTypeName() string {
	return "Applications.Connector/daprSecretStores"
}

// DaprSecretStoreProperties represents the properties of DaprSecretStore resource.
type DaprSecretStoreProperties struct {
	ProvisioningState basedatamodel.ProvisioningStates `json:"provisioningState,omitempty"`
	Environment       string                           `json:"environment"`
	Application       string                           `json:"application,omitempty"`
	Kind              DaprSecretStoreKind              `json:"kind,omitempty"`
	Type              string                           `json:"type,omitempty"`
	Version           string                           `json:"version,omitempty"`
	Metadata          map[string]interface{}           `json:"metadata,omitempty"`
}
