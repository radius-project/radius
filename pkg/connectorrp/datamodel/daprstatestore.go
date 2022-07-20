// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
)

type DaprStateStoreKind string

const (
	DaprStateStoreKindStateSqlServer    DaprStateStoreKind = "state.sqlserver"
	DaprStateStoreKindAzureTableStorage DaprStateStoreKind = "state.azure.tablestorage"
	DaprStateStoreKindGeneric           DaprStateStoreKind = "generic"
	DaprStateStoreKindUnknown           DaprStateStoreKind = "unknown"
)

// DaprStateStore represents DaprStateStore connector resource.
type DaprStateStore struct {
	v1.TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData v1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties DaprStateStoreProperties `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	v1.InternalMetadata
}

func (daprStateStore DaprStateStore) ResourceTypeName() string {
	return "Applications.Connector/daprStateStores"
}

// DaprStateStoreProperties represents the properties of DaprStateStore resource.
type DaprStateStoreProperties struct {
	v1.BasicResourceProperties
	ProvisioningState               v1.ProvisioningState                              `json:"provisioningState,omitempty"`
	Environment                     string                                            `json:"environment"`
	Application                     string                                            `json:"application,omitempty"`
	StateStoreName                  string                                            `json:"stateStoreName"`
	Kind                            DaprStateStoreKind                                `json:"kind"`
	DaprStateStoreSQLServer         DaprStateStoreSQLServerResourceProperties         `json:"daprStateStoreSQLServer"`
	DaprStateStoreAzureTableStorage DaprStateStoreAzureTableStorageResourceProperties `json:"daprStateStoreAzureTableStorage"`
	DaprStateStoreGeneric           DaprStateStoreGenericResourceProperties           `json:"daprStateStoreGeneric"`
}

type DaprStateStoreGenericResourceProperties struct {
	Metadata map[string]interface{} `json:"metadata"`
	Type     string                 `json:"type"`
	Version  string                 `json:"version"`
}

type DaprStateStoreAzureTableStorageResourceProperties struct {
	Resource string `json:"resource"`
}

type DaprStateStoreSQLServerResourceProperties struct {
	Resource string `json:"resource"`
}
