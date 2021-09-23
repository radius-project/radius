// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package inboundroutev1alpha3

import (
	"encoding/json"
	"errors"
	"fmt"
)

const (
	Kind         = "radius.dev/InboundRoute@v1alpha1"
	kindProperty = "kind"
)

type Trait struct { // TODO implement json stuff
	Kind                 string
	AdditionalProperties map[string]interface{}
}

type InboundRouteTrait struct {
	Kind     string `json:"kind"`
	Hostname string `json:"hostname"`
	Binding  string `json:"binding"`
}

// make sure to remove custom json serialization and deserialization

func FindTrait(traits []interface{}, kind string, inboundRouteTrait interface{}) (bool, error) {
	if traits == nil {
		return false, nil
	}
	for _, v := range traits {
		trait, ok := v.(Trait)
		if !ok {
			continue
		}

		if trait.Kind == kind {
			return trait.As(kind, inboundRouteTrait)
		}
	}

	return false, nil
}

func (resource Trait) As(kind string, specific interface{}) (bool, error) {
	if resource.Kind != kind {
		return false, nil
	}

	bytes, err := json.Marshal(resource)
	if err != nil {
		return false, fmt.Errorf("failed to marshal generic trait value: %w", err)
	}

	err = json.Unmarshal(bytes, specific)
	if err != nil {
		return false, fmt.Errorf("failed to unmarshal JSON as value of type %T: %w", specific, err)
	}

	return true, nil
}

func (gt Trait) MarshalJSON() ([]byte, error) {
	properties := map[string]interface{}{}
	for k, v := range gt.AdditionalProperties {
		properties[k] = v
	}

	properties[kindProperty] = gt.Kind
	return json.Marshal(properties)
}

func (gt *Trait) UnmarshalJSON(b []byte) error {
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
