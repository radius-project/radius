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

// GenericDependency is the payload for a dependsOn or provides entry in a generic form.
type GenericDependency struct {
	Name string
	Kind string

	// Absorb additional properties that are part of the dependsOn/provides.
	AdditionalProperties map[string]interface{} // JSON logic is custom, thats why there are no tags here.
}

// GenericTrait is the payload for a trait in a generic form.
type GenericTrait struct {
	Kind       string                 `json:"kind"`
	Properties map[string]interface{} `json:"properties"`
}

func (generic GenericComponent) As(kind string, specific interface{}) (bool, error) {
	if generic.Kind != kind {
		return false, nil
	}

	bytes, err := json.Marshal(generic)
	if err != nil {
		return false, fmt.Errorf("failed to marshal generic component value: %w", err)
	}

	err = json.Unmarshal(bytes, specific)
	if err != nil {
		return false, fmt.Errorf("failed to unmarshal as value of type %T: %w", specific, err)
	}

	return true, nil
}

func (generic GenericComponent) AsRequired(kind string, specific interface{}) error {
	match, err := generic.As(kind, specific)
	if err != nil {
		return err
	}

	if !match {
		return fmt.Errorf("the component was expected to have kind '%s', instead it is '%s'", kind, generic.Kind)
	}

	return nil
}

func (generic GenericComponent) FindProvidesService(name string) *GenericDependency {
	for _, p := range generic.Provides {
		if name == p.Name {
			return &p
		}
	}

	return nil
}

func (generic GenericComponent) FindProvidesServiceRequired(name string) (*GenericDependency, error) {
	provides := generic.FindProvidesService(name)
	if provides == nil {
		return nil, fmt.Errorf("the component should contain a provides service named '%s'", name)
	}

	return provides, nil
}

func (generic GenericComponent) FindTrait(kind string, trait interface{}) (bool, error) {
	for _, t := range generic.Traits {
		if kind == t.Kind {
			return t.As(kind, trait)
		}
	}

	return false, nil
}

// Since it supports 'additional' arbitrary properties, we have to implement custom JSON logic.
var _ json.Marshaler = &GenericDependency{}
var _ json.Unmarshaler = &GenericDependency{}

func (d GenericDependency) MarshalJSON() ([]byte, error) {
	values := map[string]interface{}{
		"name": d.Name,
		"kind": d.Kind,
	}

	for k, v := range d.AdditionalProperties {
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
	d.AdditionalProperties = values

	return nil
}

func (generic GenericDependency) As(kind string, specific interface{}) (bool, error) {
	if generic.Kind != kind {
		return false, nil
	}

	bytes, err := json.Marshal(generic)
	if err != nil {
		return false, fmt.Errorf("failed to marshal generic dependency value: %w", err)
	}

	err = json.Unmarshal(bytes, specific)
	if err != nil {
		return false, fmt.Errorf("failed to unmarshal JSON as value of type %T: %w", specific, err)
	}

	return true, nil
}

func (generic GenericDependency) AsRequired(kind string, specific interface{}) error {
	match, err := generic.As(kind, specific)
	if err != nil {
		return err
	} else if !match {
		return fmt.Errorf("the service was expected to have kind '%s' but was '%s", kind, generic.Kind)
	}

	return nil
}

func (generic GenericTrait) As(kind string, specific interface{}) (bool, error) {
	if generic.Kind != kind {
		return false, nil
	}

	bytes, err := json.Marshal(generic)
	if err != nil {
		return false, fmt.Errorf("failed to marshal generic trait value: %w", err)
	}

	err = json.Unmarshal(bytes, specific)
	if err != nil {
		return false, fmt.Errorf("failed to unmarshal JSON as value of type %T: %w", specific, err)
	}

	return true, nil
}
