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
	Name      string                   `json:"name"`
	Kind      string                   `json:"kind"`
	Config    map[string]interface{}   `json:"config,omitempty"`
	Run       map[string]interface{}   `json:"run,omitempty"`
	DependsOn []map[string]interface{} `json:"dependsOn,omitempty"`
	Provides  []map[string]interface{} `json:"provides,omitempty"`
	Traits    []map[string]interface{} `json:"traits,omitempty"`
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
