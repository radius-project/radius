// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	"encoding/json"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
)

// DaprPubSubBroker represents DaprPubSubBroker connector resource.
type DaprPubSubBroker struct {
	v1.TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData v1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties DaprPubSubBrokerPropertiesClassification `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	v1.InternalMetadata
}

func (daprPubSub DaprPubSubBroker) ResourceTypeName() string {
	return "Applications.Connector/daprPubSubBrokers"
}

type DaprPubSubBrokerPropertiesClassification interface {
	// GetDaprPubSubBrokerProperties returns the DaprPubSubBrokerProperties content of the underlying type.
	GetDaprPubSubBrokerProperties() DaprPubSubBrokerProperties
}

// GetDaprPubSubBrokerProperties implements the DaprPubSubBrokerPropertiesClassification interface for type DaprPubSubBrokerProperties.
func (d DaprPubSubBrokerProperties) GetDaprPubSubBrokerProperties() DaprPubSubBrokerProperties {
	return d
}

type DaprPubSubGenericResourceProperties struct {
	DaprPubSubBrokerProperties
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Type     string                 `json:"type,omitempty"`
	Version  string                 `json:"version,omitempty"`
}

type DaprPubSubAzureServiceBusResourceProperties struct {
	DaprPubSubBrokerProperties
	Resource string `json:"resource,omitempty"`
}

// DaprPubSubBrokerProperties represents the properties of DaprPubSubBroker resource.
type DaprPubSubBrokerProperties struct {
	v1.BasicResourceProperties
	ProvisioningState v1.ProvisioningState `json:"provisioningState,omitempty"`
	Environment       string               `json:"environment"`
	Application       string               `json:"application,omitempty"`
	Kind              string               `json:"kind"`
}

// UnmarshalJSON implements the json.Unmarshaller interface for type DaprPubSubBrokerResource.
func (d *DaprPubSubBroker) UnmarshalJSON(data []byte) error {
	var rawMsg map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMsg); err != nil {
		return err
	}
	for key, val := range rawMsg {
		var err error
		switch key {
		case "properties":
			d.Properties, err = unmarshalDaprPubSubBrokerPropertiesClassification(val)
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

func unmarshalDaprPubSubBrokerPropertiesClassification(rawMsg json.RawMessage) (DaprPubSubBrokerPropertiesClassification, error) {
	if rawMsg == nil {
		return nil, nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(rawMsg, &m); err != nil {
		return nil, err
	}
	var b DaprPubSubBrokerPropertiesClassification
	switch m["kind"] {
	case "generic":
		b = &DaprPubSubGenericResourceProperties{}
	case "pubsub.azure.servicebus":
		b = &DaprPubSubAzureServiceBusResourceProperties{}
	default:
		b = &DaprPubSubBrokerProperties{}
	}
	return b, json.Unmarshal(rawMsg, b)
}
