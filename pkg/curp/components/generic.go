// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package components

import (
	"encoding/json"
	"fmt"
)

// GenericComponent the payload for a component in a generic form.
type GenericComponent struct {
	Name      string                 `json:"name"`
	Kind      string                 `json:"kind"`
	Config    map[string]interface{} `json:"config,omitempty"`
	Run       map[string]interface{} `json:"run,omitempty"`
	DependsOn []GenericDependency    `json:"dependsOn,omitempty"`
	Provides  []GenericDependency    `json:"provides,omitempty"`
	Traits    []GenericTrait         `json:"traits,omitempty"`
}

type GenericDependency struct {
	// JSON logic is custom, thats why there are no tags here.
	Name  string
	Kind  string
	Extra map[string]interface{}
}

type GenericTrait struct {
	Kind       string                 `json:"kind"`
	Properties map[string]interface{} `json:"properties"`
}

// Since it supports 'additional' arbitrary properties, we have to implement custom JSON logic.
var _ json.Marshaler = &GenericDependency{}
var _ json.Unmarshaler = &GenericDependency{}

func (d GenericDependency) MarshalJSON() ([]byte, error) {
	values := map[string]interface{}{
		"name": d.Name,
		"kind": d.Kind,
	}

	for k, v := range d.Extra {
		if k == "name" || k == "kind" {
			return nil, fmt.Errorf("the property name '%s' should not appear in the extra properties", k)
		}

		values[k] = v
	}

	return json.Marshal(values)
}

func (d *GenericDependency) UnmarshalJSON(b []byte) error {
	keys := struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
	}{}
	err := json.Unmarshal(b, &keys)
	if err != nil {
		return err
	}

	values := map[string]interface{}{}
	err = json.Unmarshal(b, &values)
	if err != nil {
		return err
	}

	d.Name = keys.Name
	d.Kind = keys.Kind

	delete(values, "name")
	delete(values, "kind")
	d.Extra = values

	return nil
}

func ConvertFromGeneric(generic GenericComponent, specific interface{}) error {
	bytes, err := json.Marshal(generic)
	if err != nil {
		return fmt.Errorf("failed to marshal generic component value: %w", err)
	}

	err = json.Unmarshal(bytes, specific)
	if err != nil {
		return fmt.Errorf("failed to unmarshal as value of type %T: %w", specific, err)
	}

	return err
}
