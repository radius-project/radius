// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package components

import (
	"encoding/json"
	"fmt"
)

// GenericComponent represents a binding used by an Radius Component.
type GenericComponent struct {
	Name     string                    `json:"name"`
	Kind     string                    `json:"kind"`
	Config   map[string]interface{}    `json:"config,omitempty"`
	Run      map[string]interface{}    `json:"run,omitempty"`
	Bindings map[string]GenericBinding `json:"provides,omitempty"`
	Uses     []GenericDependency       `json:"uses,omitempty"`
	Traits   []GenericTrait            `json:"traits,omitempty"`
}

// GenericBinding represents a binding provided by an Radius Component in a generic form.
type GenericBinding struct {
	Kind                 string
	AdditionalProperties map[string]interface{}

	// GenericBinding has custom marshaling code
}

// GenericDependency represents a binding used by an Radius Component.
type GenericDependency struct {
	Binding BindingExpression            `json:"binding"`
	Env     map[string]BindingExpression `json:"env,omitempty"`
	Secrets *GenericDependencySecrets    `json:"secrets,omitempty"`
}

// GenericDependencySecrets represents actions to take on a secret store as part of a binding.
type GenericDependencySecrets struct {
	Store BindingExpression            `json:"store"`
	Keys  map[string]BindingExpression `json:"keys,omitempty"`
}

// GenericTrait represents a trait for an Radius component.
type GenericTrait struct {
	Kind                 string                 `json:"kind"`
	AdditionalProperties map[string]interface{} `json:"additionalProperties,omitempty"`

	// GenericTrait has custom marshaling code
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

func (generic GenericComponent) FindTrait(kind string, trait interface{}) (bool, error) {
	for _, t := range generic.Traits {
		if kind == t.Kind {
			return t.As(kind, trait)
		}
	}

	return false, nil
}

func (generic GenericBinding) As(kind string, specific interface{}) (bool, error) {
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

func (generic GenericBinding) AsRequired(kind string, specific interface{}) error {
	match, err := generic.As(kind, specific)
	if err != nil {
		return err
	} else if !match {
		return fmt.Errorf("the binding was expected to have kind '%s' but was '%s", kind, generic.Kind)
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
