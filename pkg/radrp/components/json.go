// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package components

import (
	"encoding/json"
	"errors"
)

const kindProperty = "kind"

func (gb GenericBinding) MarshalJSON() ([]byte, error) {
	properties := map[string]interface{}{}
	for k, v := range gb.AdditionalProperties {
		properties[k] = v
	}

	properties[kindProperty] = gb.Kind
	return json.Marshal(properties)
}

func (gb *GenericBinding) UnmarshalJSON(b []byte) error {
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

	gb.Kind = kind
	gb.AdditionalProperties = properties
	return nil
}

func (gt GenericTrait) MarshalJSON() ([]byte, error) {
	properties := map[string]interface{}{}
	for k, v := range gt.AdditionalProperties {
		properties[k] = v
	}

	properties[kindProperty] = gt.Kind
	return json.Marshal(properties)
}

func (gt *GenericTrait) UnmarshalJSON(b []byte) error {
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

	gt.Kind = kind
	gt.AdditionalProperties = properties
	return nil
}
