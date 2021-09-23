// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package inboundroutev1alpha3

import (
	"encoding/json"
	"fmt"
)

const Kind = "radius.dev/InboundRoute@v1alpha1"

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
