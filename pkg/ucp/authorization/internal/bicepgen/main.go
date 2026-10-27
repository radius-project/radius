// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

// Command bicepgen adds Radius authorization placement metadata that cannot be
// inferred from ARM resource kinds by the shared TypeSpec Bicep emitter.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type node map[string]json.RawMessage

func main() {
	path := flag.String("types", "", "Radius.Core generated types.json")
	flag.Parse()
	if err := run(*path); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(path string) error {
	if path == "" {
		return fmt.Errorf("--types is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	result, err := customize(raw)
	if err != nil {
		return err
	}
	return os.WriteFile(path, result, 0600)
}

func field[T any](value node, key string) (T, error) {
	var result T
	err := json.Unmarshal(value[key], &result)
	if err != nil {
		return result, fmt.Errorf("read Bicep %s: %w", key, err)
	}
	return result, nil
}

func set(value node, key string, input any) error {
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	value[key] = raw
	return nil
}

func reference(index int) node {
	return node{"$ref": json.RawMessage(strconv.Quote("#/" + strconv.Itoa(index)))}
}

func customize(raw []byte) ([]byte, error) {
	var types []node
	if err := json.Unmarshal(raw, &types); err != nil {
		return nil, err
	}
	var scope node
	found := map[string]bool{}
	for _, resource := range types {
		name, ok := resource["name"]
		if !ok {
			continue
		}
		var typeName string
		if err := json.Unmarshal(name, &typeName); err != nil {
			return nil, err
		}
		base, _, _ := strings.Cut(typeName, "@")
		if base != "Radius.Core/roleDefinitions" && base != "Radius.Core/roleAssignments" {
			continue
		}
		kind, err := field[string](resource, "$type")
		if err != nil {
			return nil, err
		}
		if kind != "ResourceType" {
			continue
		}
		body, err := field[map[string]string](resource, "body")
		if err != nil {
			return nil, err
		}
		index, err := strconv.Atoi(strings.TrimPrefix(body["$ref"], "#/"))
		if err != nil || index < 0 || index >= len(types) {
			return nil, fmt.Errorf("invalid Bicep policy body reference")
		}
		properties, err := field[map[string]node](types[index], "properties")
		if err != nil {
			return nil, err
		}
		if scope == nil {
			if existing, ok := properties["scope"]; ok {
				scope, err = field[node](existing, "type")
				if err != nil {
					return nil, err
				}
			} else {
				stringIndex := len(types)
				types = append(types, node{"$type": json.RawMessage(`"StringType"`)})
				objectIndex := len(types)
				object := node{"$type": json.RawMessage(`"ObjectType"`), "name": json.RawMessage(`"RadiusAuthorizationResourceScope"`)}
				if err := set(object, "properties", map[string]any{"id": map[string]any{"type": reference(stringIndex), "flags": 1}}); err != nil {
					return nil, err
				}
				types = append(types, object)
				scope = reference(len(types))
				union := node{"$type": json.RawMessage(`"UnionType"`)}
				if err := set(union, "elements", []node{reference(stringIndex), reference(objectIndex)}); err != nil {
					return nil, err
				}
				types = append(types, union)
			}
		}
		property := node{"flags": json.RawMessage("0"), "description": json.RawMessage(`"Assignment target resource or ID; existing roles may select an ancestor scope. Omit for the deployment resource group."`)}
		if err := set(property, "type", scope); err != nil {
			return nil, err
		}
		properties["scope"] = property
		if base == "Radius.Core/roleAssignments" {
			// Radius extension IDs are runtime outputs. Unlike Azure resources,
			// a deterministic assignment name may depend on the role's ID.
			flags, err := field[int](properties["name"], "flags")
			if err != nil {
				return nil, err
			}
			if err := set(properties["name"], "flags", flags&^8); err != nil {
				return nil, err
			}
			for _, key := range []string{"readableScopes", "writableScopes"} {
				if err := set(resource, key, 0); err != nil {
					return nil, err
				}
			}
		}
		if err := set(types[index], "properties", properties); err != nil {
			return nil, err
		}
		found[base] = true
	}
	if len(found) != 2 {
		return nil, fmt.Errorf("authorization role and assignment types were not both generated")
	}
	result, err := json.MarshalIndent(types, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(result, '\n'), nil
}
