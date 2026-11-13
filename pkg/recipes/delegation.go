/*
Copyright 2026 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package recipes

import (
	"fmt"
	"strings"

	"github.com/radius-project/radius/pkg/ucp/resources"
)

// IncomingReference supplies a delegating resource's desired inputs and revision to its target recipe.
type IncomingReference struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	APIVersion string         `json:"apiVersion"`
	Revision   string         `json:"revision"`
	Properties map[string]any `json:"properties"`
}

// DelegatedOutput returns computed values for one delegating resource's desired revision.
type DelegatedOutput struct {
	ID       string         `json:"id"`
	Revision string         `json:"revision"`
	Values   map[string]any `json:"values"`
}

// ValidateIncomingReferences checks identities and revisions without exposing desired values.
// The caller is responsible for schema validation, secret protection, and target consent.
func ValidateIncomingReferences(incoming []IncomingReference) error {
	seen := make(map[string]struct{}, len(incoming))
	for i, reference := range incoming {
		id, err := resources.ParseResource(reference.ID)
		if err != nil || id.FindScope("radius") == "" || !strings.EqualFold(id.Type(), reference.Type) {
			return fmt.Errorf("incoming reference %d must identify a Radius resource of its declared type", i)
		}
		key := strings.ToLower(id.String())
		if _, exists := seen[key]; exists {
			return fmt.Errorf("incoming references contain a duplicate Radius resource ID")
		}
		seen[key] = struct{}{}
		if strings.TrimSpace(reference.APIVersion) == "" || strings.TrimSpace(reference.Revision) == "" || reference.Properties == nil {
			return fmt.Errorf("incoming reference %d requires an API version, desired-state revision, and desired properties", i)
		}
	}
	return nil
}

// ValidateDelegatedOutputs requires exactly one computed result per incoming desired revision.
// Source-schema validation must also succeed before the caller publishes any computed properties.
func (ro *RecipeOutput) ValidateDelegatedOutputs(incoming []IncomingReference) error {
	if err := ValidateIncomingReferences(incoming); err != nil {
		return err
	}
	expected := make(map[string]IncomingReference, len(incoming))
	for _, reference := range incoming {
		expected[strings.ToLower(reference.ID)] = reference
	}
	seen := make(map[string]struct{}, len(ro.DelegatedOutputs))
	for i, output := range ro.DelegatedOutputs {
		key := strings.ToLower(output.ID)
		reference, exists := expected[key]
		if !exists {
			return fmt.Errorf("delegated output %d does not identify an incoming Radius resource", i)
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("delegated outputs contain a duplicate Radius resource ID")
		}
		seen[key] = struct{}{}
		if output.Revision != reference.Revision {
			return fmt.Errorf("delegated output %d does not match its desired-state revision", i)
		}
		if output.Values == nil {
			return fmt.Errorf("delegated output %d requires a computed values object", i)
		}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("recipe result is missing delegated outputs")
	}
	return nil
}
