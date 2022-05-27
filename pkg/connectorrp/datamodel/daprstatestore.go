// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	"encoding/json"

	"github.com/project-radius/radius/pkg/api/armrpcv1"
	"github.com/project-radius/radius/pkg/basedatamodel"
)

// DaprStateStore represents DaprStateStore connector resource.
type DaprStateStore struct {
	basedatamodel.TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData armrpcv1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties DaprStateStorePropertiesClassification `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	basedatamodel.InternalMetadata
}

type DaprStateStorePropertiesClassification interface {
	// GetDaprStateStoreProperties returns the DaprStateStoreProperties content of the underlying type.
	GetDaprStateStoreProperties() DaprStateStoreProperties
}

// GetDaprStateStoreProperties implements the DaprStateStorePropertiesClassification interface for type DaprStateStoreProperties.
func (d DaprStateStoreProperties) GetDaprStateStoreProperties() DaprStateStoreProperties { return d }

func (daprStateStore DaprStateStore) ResourceTypeName() string {
	return "Applications.Connector/daprStateStores"
}

// DaprStateStoreProperties represents the properties of DaprStateStore resource.
type DaprStateStoreProperties struct {
	basedatamodel.BasicResourceProperties
	ProvisioningState basedatamodel.ProvisioningStates `json:"provisioningState,omitempty"`
	Environment       string                           `json:"environment"`
	Application       string                           `json:"application,omitempty"`
	Kind              string                           `json:"kind"`
}

type DaprStateStoreGenericResourceProperties struct {
	DaprStateStoreProperties
	Metadata map[string]interface{} `json:"metadata"`
	Type     string                 `json:"type"`
	Version  string                 `json:"version"`
}

type DaprStateStoreAzureTableStorageResourceProperties struct {
	DaprStateStoreProperties
	Resource string `json:"resource"`
}

type DaprStateStoreSQLServerResourceProperties struct {
	DaprStateStoreProperties
	Resource string `json:"resource"`
}

// UnmarshalJSON implements the json.Unmarshaller interface for type DaprStateStoreResource.
func (d *DaprStateStore) UnmarshalJSON(data []byte) error {
	var rawMsg map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMsg); err != nil {
		return err
	}
	for key, val := range rawMsg {
		var err error
		switch key {
		case "properties":
			d.Properties, err = unmarshalDaprStateStorePropertiesClassification(val)
			delete(rawMsg, key)
		case "systemData":
			err = unpopulate(val, &d.SystemData)
			delete(rawMsg, key)
		case "id":
			err = unpopulate(val, &d.TrackedResource.ID)
			delete(rawMsg, key)
		case "name":
			err = unpopulate(val, &d.TrackedResource.Name)
			delete(rawMsg, key)
		case "type":
			err = unpopulate(val, &d.TrackedResource.Type)
			delete(rawMsg, key)
		case "location":
			err = unpopulate(val, &d.TrackedResource.Location)
			delete(rawMsg, key)
		case "tags":
			err = unpopulate(val, &d.TrackedResource.Tags)
			delete(rawMsg, key)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func unmarshalDaprStateStorePropertiesClassification(rawMsg json.RawMessage) (DaprStateStorePropertiesClassification, error) {
	if rawMsg == nil {
		return nil, nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(rawMsg, &m); err != nil {
		return nil, err
	}
	var b DaprStateStorePropertiesClassification
	switch m["kind"] {
	case "generic":
		b = &DaprStateStoreGenericResourceProperties{}
	case "state.azure.tablestorage":
		b = &DaprStateStoreAzureTableStorageResourceProperties{}
	case "state.sqlserver":
		b = &DaprStateStoreSQLServerResourceProperties{}
	default:
		b = &DaprStateStoreProperties{}
	}
	return b, json.Unmarshal(rawMsg, b)
}

func unpopulate(data json.RawMessage, v interface{}) error {
	if data == nil {
		return nil
	}
	return json.Unmarshal(data, v)
}
