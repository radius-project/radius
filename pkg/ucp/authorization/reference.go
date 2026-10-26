// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/radius-project/radius/pkg/ucp/resources"
)

// ReferenceOptions supplies resolved reference descriptors for the exact
// registered API version and credential planes. Plane selection is a trusted caller responsibility:
// the evaluator cannot infer a cloud plane from an Azure subscription or AWS account.
type ReferenceOptions struct {
	References  []ReferenceDescriptor
	CloudPlanes map[string]string
}

// ReferenceDescriptor is an internal, resolved schema reference, not a public
// x-radius-reference wire format. The schema adapter must resolve $ref and
// conditional branches before producing these descriptors. "*" traverses each
// array element or dictionary value. Required rejects a missing or empty path.
type ReferenceDescriptor struct {
	Path         []string
	ResourceType string
	Required     bool
}

// ReferenceChecks extracts standard environment/application references on
// built-in and registered resources, built-in connections, and schema-declared
// references on PUT/PATCH, including unchanged references. For PATCH the caller
// must supply the complete effective resource after applying the patch, not
// just its changed fields. It does not read target resources or follow their
// references. Reads/deletes require no reference checks.
// Required marked references and malformed values fail closed. Public
// x-radius-reference syntax and the registration schema adapter are not defined here.
func (c *Catalog) ReferenceChecks(method, target string, body map[string]any, options ReferenceOptions) ([]Check, error) {
	if method == http.MethodGet || method == http.MethodDelete {
		return nil, nil
	}
	if method != http.MethodPut && method != http.MethodPatch {
		return nil, fmt.Errorf("unsupported reference-check method %q", method)
	}
	canonical, err := canonicalTarget(target)
	if err != nil {
		return nil, err
	}
	id, err := resources.Parse(canonical)
	if err != nil || !id.IsResource() {
		return nil, fmt.Errorf("reference checks require a resource target")
	}
	if body == nil {
		return nil, fmt.Errorf("request body is required")
	}
	checks := []Check{}
	add := func(value any, operation string, expected []string) error {
		reference, ok := value.(string)
		if !ok || reference == "" {
			return fmt.Errorf("reference must be a nonempty concrete resource ID")
		}
		canonical, err := canonicalTarget(reference)
		if err != nil {
			return err
		}
		referenced, err := resources.Parse(canonical)
		if err != nil || !referenced.IsResource() {
			return fmt.Errorf("reference is not a resource ID")
		}
		if len(expected) > 0 && !containsFold(expected, referenced.Type()) {
			return fmt.Errorf("reference has unexpected type %q", referenced.Type())
		}
		permission := referenced.Type() + "/" + operation
		if err := c.ValidatePermissions([]string{permission}); err != nil {
			return err
		}
		checks = append(checks, Check{c.permissions[strings.ToLower(permission)], canonical})
		return nil
	}
	typ := strings.ToLower(id.Type())
	isBuiltIn := strings.HasPrefix(typ, "applications.") ||
		strings.HasPrefix(typ, "radius.") && !strings.HasPrefix(typ, "radius.core/") ||
		containsFold([]string{"Radius.Core/applications", "Radius.Core/environments", "Radius.Core/recipePacks"}, typ)
	if isBuiltIn || c.registered[typ] {
		if value, exists := body["properties"]; exists {
			properties, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("properties must be an object")
			}
			for _, reference := range []struct {
				property, operation string
				types               []string
			}{
				{"environment", "use/action", []string{"Radius.Core/environments", "Applications.Core/environments"}},
				{"application", "write", []string{"Radius.Core/applications", "Applications.Core/applications"}},
			} {
				if value, exists := properties[reference.property]; exists {
					if err := add(value, reference.operation, reference.types); err != nil {
						return nil, fmt.Errorf("properties.%s: %w", reference.property, err)
					}
				}
			}
			if value, exists := properties["connections"]; exists && isBuiltIn {
				connections, ok := value.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("connections must be an object")
				}
				for _, name := range sortedKeys(connections) {
					connection, ok := connections[name].(map[string]any)
					if !ok {
						return nil, fmt.Errorf("connection %q must be an object", name)
					}
					if err := add(connection["source"], "connect/action", nil); err != nil {
						return nil, fmt.Errorf("connection %q: %w", name, err)
					}
				}
			}
			if containsFold([]string{"Radius.Core/environments", "Applications.Core/environments"}, typ) {
				if value, exists := properties["recipePacks"]; exists {
					packs, ok := value.([]any)
					if !ok {
						return nil, fmt.Errorf("recipePacks must be an array")
					}
					for _, pack := range packs {
						if err := add(pack, "use/action", []string{"Radius.Core/recipePacks"}); err != nil {
							return nil, fmt.Errorf("recipePacks: %w", err)
						}
					}
				}
				if value, exists := properties["providers"]; exists {
					providers, ok := value.(map[string]any)
					if !ok {
						return nil, fmt.Errorf("providers must be an object")
					}
					for _, cloud := range []string{"azure", "aws"} {
						if provider, exists := providers[cloud]; exists {
							if _, ok := provider.(map[string]any); !ok {
								return nil, fmt.Errorf("provider %s must be an object", cloud)
							}
							plane, err := canonicalTarget(options.CloudPlanes[cloud])
							if err != nil {
								return nil, fmt.Errorf("resolve %s credential plane: %w", cloud, err)
							}
							parsed, err := resources.Parse(plane)
							if err != nil || !parsed.IsScope() || len(parsed.ScopeSegments()) != 1 ||
								!strings.EqualFold(parsed.ScopeSegments()[0].Type, cloud) {
								return nil, fmt.Errorf("invalid %s credential plane", cloud)
							}
							namespace := map[string]string{"azure": "Azure", "aws": "AWS"}[cloud]
							checks = append(checks, Check{"System." + namespace + "/credentials/use/action", plane})
						}
					}
				}
			}
		}
	}
	for _, descriptor := range options.References {
		if len(descriptor.Path) == 0 || len(descriptor.Path) > 64 || !resourceTypePattern.MatchString(descriptor.ResourceType) {
			return nil, fmt.Errorf("invalid resolved reference descriptor")
		}
		if err := walkReference(descriptor, descriptor.Path, body, add); err != nil {
			return nil, err
		}
	}
	slices.SortFunc(checks, func(a, b Check) int {
		return strings.Compare(a.Target+" "+a.Permission, b.Target+" "+b.Permission)
	})
	return slices.Compact(checks), nil
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for name := range values {
		keys = append(keys, name)
	}
	slices.Sort(keys)
	return keys
}

func walkReference(descriptor ReferenceDescriptor, path []string, value any, add func(any, string, []string) error) error {
	if len(path) == 0 {
		return add(value, "read", []string{descriptor.ResourceType})
	}
	if path[0] == "*" {
		switch collection := value.(type) {
		case []any:
			if descriptor.Required && len(collection) == 0 {
				return fmt.Errorf("AuthorizationIndeterminate: required reference collection is empty")
			}
			for _, child := range collection {
				if err := walkReference(descriptor, path[1:], child, add); err != nil {
					return err
				}
			}
		case map[string]any:
			if descriptor.Required && len(collection) == 0 {
				return fmt.Errorf("AuthorizationIndeterminate: required reference dictionary is empty")
			}
			for _, name := range sortedKeys(collection) {
				if err := walkReference(descriptor, path[1:], collection[name], add); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("reference path wildcard requires an array or dictionary")
		}
		return nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("reference path %q requires an object", path[0])
	}
	child, exists := object[path[0]]
	if !exists {
		if descriptor.Required {
			return fmt.Errorf("AuthorizationIndeterminate: required reference %q is missing", strings.Join(descriptor.Path, "."))
		}
		return nil
	}
	return walkReference(descriptor, path[1:], child, add)
}
