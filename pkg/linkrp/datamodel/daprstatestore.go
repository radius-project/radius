// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/rp"
)

type DaprStateStoreKind string

const (
	DaprStateStoreKindStateSqlServer    DaprStateStoreKind = "state.sqlserver"
	DaprStateStoreKindAzureTableStorage DaprStateStoreKind = "state.azure.tablestorage"
	DaprStateStoreKindGeneric           DaprStateStoreKind = "generic"
	DaprStateStoreKindUnknown           DaprStateStoreKind = "unknown"
)

type DaprStateStoreMode string

const (
	DaprStateStoreModeRecipe   DaprStateStoreMode = "recipe"
	DaprStateStoreModeResource DaprStateStoreMode = "resource"
	DaprStateStoreModeValues   DaprStateStoreMode = "values"
)

// DaprStateStore represents DaprStateStore link resource.
type DaprStateStore struct {
	v1.TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData v1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties DaprStateStoreProperties `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	v1.InternalMetadata

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

func (daprStateStore DaprStateStore) ResourceTypeName() string {
	return "Applications.Link/daprStateStores"
}

// DaprStateStoreProperties represents the properties of DaprStateStore resource.
type DaprStateStoreProperties struct {
	rp.BasicResourceProperties
	rp.BasicDaprResourceProperties
	ProvisioningState v1.ProvisioningState   `json:"provisioningState,omitempty"`
	Kind              DaprStateStoreKind     `json:"kind,omitempty"`
	Mode              DaprStateStoreMode     `json:"mode,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	Recipe            LinkRecipe             `json:"recipe,omitempty"`
	Resource          string                 `json:"resource,omitempty"`
	Type              string                 `json:"type,omitempty"`
	Version           string                 `json:"version,omitempty"`
}
