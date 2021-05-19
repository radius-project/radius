// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rest

import (
	"encoding/json"
	"errors"
)

const kindProperty = "kind"

func (cb ComponentBinding) MarshalJSON() ([]byte, error) {
	properties := map[string]interface{}{}
	for k, v := range cb.AdditionalProperties {
		properties[k] = v
	}

	properties[kindProperty] = cb.Kind
	return json.Marshal(properties)
}

func (cb *ComponentBinding) UnmarshalJSON(b []byte) error {
	properties := map[string]interface{}{}
	err := json.Unmarshal(b, &properties)
	if err != nil {
		return err
	}

	obj, ok := properties[kindProperty]
	if !ok {
		return errors.New("the 'kind' property is required")
	}

	kind, ok := obj.(string)
	if !ok {
		return errors.New("the 'kind' property must be a string")
	}

	delete(properties, kindProperty)

	cb.Kind = kind
	cb.AdditionalProperties = properties
	return nil
}

func (ct ComponentTrait) MarshalJSON() ([]byte, error) {
	properties := map[string]interface{}{}
	for k, v := range ct.AdditionalProperties {
		properties[k] = v
	}

	properties[kindProperty] = ct.Kind
	return json.Marshal(properties)
}

func (ct *ComponentTrait) UnmarshalJSON(b []byte) error {
	properties := map[string]interface{}{}
	err := json.Unmarshal(b, &properties)
	if err != nil {
		return err
	}

	obj, ok := properties[kindProperty]
	if !ok {
		return errors.New("the 'kind' property is required")
	}

	kind, ok := obj.(string)
	if !ok {
		return errors.New("the 'kind' property must be a string")
	}

	delete(properties, kindProperty)

	ct.Kind = kind
	ct.AdditionalProperties = properties
	return nil
}
