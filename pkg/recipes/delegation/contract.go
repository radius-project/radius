// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

// Package delegation implements the experimental, single-writer recipe contract.
package delegation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// EnabledVariable is deliberately opt-in: this prototype is not a production scheduler.
const EnabledVariable = "RADIUS_EXPERIMENTAL_DELEGATION"

// Kind identifies registrations which execute no driver.
const Kind = "delegated"

var selectorPattern = regexp.MustCompile(`^\$\.[A-Za-z_][A-Za-z0-9_]*(\[\*\])?(\.[A-Za-z_][A-Za-z0-9_]*(\[\*\])?)*$`)

// ValidatePack validates the prototype's same-pack, one-hop registration contract.
func ValidatePack(definitions map[string]*datamodel.RecipeDefinition, enabled bool) error {
	for resourceType, definition := range definitions {
		if definition == nil {
			return fmt.Errorf("recipe %q has no definition", resourceType)
		}
		if definition.Kind != Kind {
			if definition.Source == "" {
				return fmt.Errorf("direct recipe %q requires source", resourceType)
			}
			if definition.Target != nil {
				return fmt.Errorf("direct recipe %q cannot select a delegation target", resourceType)
			}
			if len(definition.AcceptsDelegationFrom) > 0 && !enabled {
				return fmt.Errorf("delegated provisioning requires %s=true", EnabledVariable)
			}
			continue
		}
		if !enabled {
			return fmt.Errorf("delegated provisioning requires %s=true", EnabledVariable)
		}
		if definition.Target == nil || !selectorPattern.MatchString(definition.Target.IDsFrom) {
			return fmt.Errorf("delegated recipe %q requires a field/wildcard target selector", resourceType)
		}
		if definition.Source != "" || len(definition.Parameters)+len(definition.Outputs)+len(definition.SecretOutputs)+len(definition.AcceptsDelegationFrom) > 0 || definition.PlainHTTP {
			return fmt.Errorf("delegated recipe %q cannot have execution configuration", resourceType)
		}
		var target *datamodel.RecipeDefinition
		for name, candidate := range definitions {
			if strings.EqualFold(name, definition.Target.ResourceType) {
				target = candidate
			}
		}
		if target == nil || target.Kind == Kind || strings.EqualFold(resourceType, definition.Target.ResourceType) {
			return fmt.Errorf("delegated recipe %q requires a direct target in the same pack", resourceType)
		}
		if !Accepts(target.AcceptsDelegationFrom, resourceType) {
			return fmt.Errorf("target recipe does not accept delegation from %q", resourceType)
		}
	}
	return nil
}

// Accepts matches resource types case-insensitively.
func Accepts(types []string, resourceType string) bool {
	return slices.ContainsFunc(types, func(t string) bool { return strings.EqualFold(t, resourceType) })
}

// SelectTarget evaluates only field names and [*], canonicalizes IDs, and requires one target.
func SelectTarget(properties map[string]any, target *datamodel.DelegationTarget) (string, error) {
	if target == nil || !selectorPattern.MatchString(target.IDsFrom) {
		return "", fmt.Errorf("invalid delegation selector")
	}
	values := []any{properties}
	for _, field := range strings.Split(strings.TrimPrefix(target.IDsFrom, "$."), ".") {
		array := strings.HasSuffix(field, "[*]")
		field = strings.TrimSuffix(field, "[*]")
		next := []any{}
		for _, value := range values {
			object, ok := value.(map[string]any)
			if !ok {
				return "", fmt.Errorf("selector field %q requires an object", field)
			}
			value, exists := object[field]
			if !exists {
				return "", fmt.Errorf("selector field %q is missing", field)
			}
			if array {
				items, ok := value.([]any)
				if !ok {
					return "", fmt.Errorf("selector field %q requires an array", field)
				}
				next = append(next, items...)
			} else {
				next = append(next, value)
			}
		}
		values = next
	}
	ids := map[string]bool{}
	for _, value := range values {
		rawID, ok := value.(string)
		if !ok {
			return "", fmt.Errorf("selector must produce Radius resource IDs")
		}
		id, err := resources.ParseResource(rawID)
		if err != nil {
			return "", fmt.Errorf("invalid delegation target ID: %w", err)
		}
		if !strings.EqualFold(id.Type(), target.ResourceType) {
			return "", fmt.Errorf("selected target has type %q, expected %q", id.Type(), target.ResourceType)
		}
		ids[strings.ToLower(id.String())] = true
	}
	if len(ids) != 1 {
		return "", fmt.Errorf("delegation requires exactly one target, selected %d", len(ids))
	}
	for id := range ids {
		return id, nil
	}
	panic("unreachable")
}

// Revision hashes JSON deterministically; it is unaffected by status-only writes.
func Revision(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// DesiredProperties copies schema-declared writable inputs without framework state.
// The prototype deliberately rejects sensitive inputs and complex schema composition.
func DesiredProperties(properties, schema map[string]any) (map[string]any, error) {
	value, err := desiredValue(properties, schema)
	if err != nil {
		return nil, err
	}
	result, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("desired properties must be an object")
	}
	for _, key := range []string{"status", "provisioningState", "secrets", "recipe"} {
		delete(result, key)
	}
	return result, nil
}

func desiredValue(value any, schema map[string]any) (any, error) {
	for _, key := range []string{"$ref", "allOf", "anyOf", "oneOf"} {
		if _, exists := schema[key]; exists {
			return nil, fmt.Errorf("experimental delegation does not support composed schemas")
		}
	}
	if sensitive, _ := schema["x-radius-sensitive"].(bool); sensitive {
		return nil, fmt.Errorf("experimental delegation does not support sensitive inputs")
	}
	switch value := value.(type) {
	case map[string]any:
		result := map[string]any{}
		fields, _ := schema["properties"].(map[string]any)
		for key, fieldValue := range value {
			fieldSchema, _ := fields[key].(map[string]any)
			if fieldSchema == nil {
				fieldSchema, _ = schema["additionalProperties"].(map[string]any)
			}
			if fieldSchema == nil {
				continue
			}
			if readOnly, _ := fieldSchema["readOnly"].(bool); readOnly {
				continue
			}
			next, err := desiredValue(fieldValue, fieldSchema)
			if err != nil {
				return nil, err
			}
			result[key] = next
		}
		return result, nil
	case []any:
		result := make([]any, 0, len(value))
		itemSchema, _ := schema["items"].(map[string]any)
		for _, item := range value {
			next, err := desiredValue(item, itemSchema)
			if err != nil {
				return nil, err
			}
			result = append(result, next)
		}
		return result, nil
	default:
		return value, nil
	}
}

// ValidateOutputs enforces an exact snapshot ID/revision match before any projection.
func ValidateOutputs(incoming []recipes.IncomingReference, outputs []recipes.DelegatedOutput) error {
	if len(incoming) != len(outputs) {
		return fmt.Errorf("delegatedOutputs must contain exactly one entry per incoming reference")
	}
	expected := map[string]string{}
	for _, reference := range incoming {
		expected[strings.ToLower(reference.ID)] = reference.Revision
	}
	seen := map[string]bool{}
	for _, output := range outputs {
		id := strings.ToLower(output.ID)
		revision, exists := expected[id]
		if !exists || seen[id] || output.Revision != revision {
			return fmt.Errorf("delegated output has a foreign, duplicate, or stale ID/revision")
		}
		seen[id] = true
	}
	return nil
}
