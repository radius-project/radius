// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	"encoding/json"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
)

// Extender represents Extender connector resource.
type Extender struct {
	v1.TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData v1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties ExtenderProperties `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	v1.InternalMetadata
}

func (extender Extender) ResourceTypeName() string {
	return "Applications.Connector/extenders"
}

// ExtenderProperties represents the properties of Extender resource.
type ExtenderProperties struct {
	v1.BasicResourceProperties
	AdditionalProperties map[string]interface{}
	ProvisioningState    v1.ProvisioningState   `json:"provisioningState,omitempty"`
	Environment          string                 `json:"environment"`
	Application          string                 `json:"application,omitempty"`
	Secrets              map[string]interface{} `json:"secrets,omitempty"`
}

// UnmarshalJSON implements the json.Unmarshaller interface for type ExtenderProperties.
func (e *ExtenderProperties) UnmarshalJSON(data []byte) error {
	var rawMsg map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMsg); err != nil {
		return err
	}
	for key, val := range rawMsg {
		var err error
		switch key {
		case "application":
			err = unpopulate(val, &e.Application)
			delete(rawMsg, key)
		case "environment":
			err = unpopulate(val, &e.Environment)
			delete(rawMsg, key)
		case "provisioningState":
			err = unpopulate(val, &e.ProvisioningState)
			delete(rawMsg, key)
		case "secrets":
			err = unpopulate(val, &e.Secrets)
			delete(rawMsg, key)
		}
		if err != nil {
			return err
		}
	}
	if err := unmarshalInternal(&e.BasicResourceProperties, rawMsg); err != nil {
		return err
	}
	for key, val := range rawMsg {
		var err error
		if e.AdditionalProperties == nil {
			e.AdditionalProperties = map[string]interface{}{}
		}
		if val != nil {
			var aux interface{}
			err = json.Unmarshal(val, &aux)
			e.AdditionalProperties[key] = aux
		}
		delete(rawMsg, key)
		if err != nil {
			return err
		}
	}
	return nil
}

func unmarshalInternal(b *v1.BasicResourceProperties, rawMsg map[string]json.RawMessage) error {
	for key, val := range rawMsg {
		var err error
		switch key {
		case "status":
			err = unpopulate(val, &b.Status)
			delete(rawMsg, key)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
