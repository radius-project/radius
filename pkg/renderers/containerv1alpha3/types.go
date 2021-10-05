// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containerv1alpha3

import (
	"encoding/json"
	"fmt"
)

const (
	kindProperty = "kind"
	ResourceType = "ContainerComponent"
)

// ContainerProperties represents the 'properties' node of a ContainerComponent resource.
type ContainerProperties struct {
	Container   Container                      `json:"container,omitempty"`
	Connections map[string]ContainerConnection `json:"connections,omitempty"`
	Traits      []ContainerTrait               `json:"traits,omitempty"`
}

type Container struct {
	Image   string                            `json:"image"`
	Ports   map[string]ContainerPort          `json:"ports,omitempty"`
	Env     map[string]interface{}            `json:"env,omitempty"`
	Volumes map[string]map[string]interface{} `json:"volumes,omitempty"`
}

type ContainerPort struct {
	Provides      string `json:"provides"`
	Protocol      string `json:"protocol"`
	ContainerPort *int   `json:"containerPort"`
}

type ContainerConnection struct {
	Kind   string `json:"kind"`
	Source string `json:"source"`
}

type ContainerTrait struct {
	Kind                 string
	AdditionalProperties map[string]interface{}
}

type EphemeralVolume struct {
	Kind         string `json:"kind"`
	MountPath    string `json:"mountPath"`
	ManagedStore string `json:"managedStore"`
}

func (ct ContainerTrait) MarshalJSON() ([]byte, error) {
	properties := map[string]interface{}{}
	for k, v := range ct.AdditionalProperties {
		properties[k] = v
	}

	properties[kindProperty] = ct.Kind
	return json.Marshal(properties)
}

func (ct *ContainerTrait) UnmarshalJSON(b []byte) error {
	properties := map[string]interface{}{}
	err := json.Unmarshal(b, &properties)
	if err != nil {
		return err
	}

	obj, ok := properties[kindProperty]
	if !ok {
		return fmt.Errorf("the '%s' property is required", kindProperty)
	}

	kind, ok := obj.(string)
	if !ok {
		return fmt.Errorf("the '%s' property must be a string", kindProperty)
	}

	delete(properties, kindProperty)

	ct.Kind = kind
	ct.AdditionalProperties = properties
	return nil
}

func (resource ContainerProperties) FindTrait(kind string, trait interface{}) (bool, error) {
	traits := resource.Traits
	if traits == nil {
		return false, nil
	}
	for _, v := range traits {
		if v.Kind == kind {
			return v.As(kind, trait)
		}
	}

	return false, nil
}

func (resource ContainerTrait) As(kind string, specific interface{}) (bool, error) {
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
