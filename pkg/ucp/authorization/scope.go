// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/radius-project/radius/pkg/ucp/resources"
)

func canonicalTarget(value string) (string, error) {
	if value == "/" {
		return value, nil
	}
	if !utf8.ValidString(value) || !strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") {
		return "", fmt.Errorf("invalid target %q", value)
	}
	for _, part := range strings.Split(value[1:], "/") {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, "%\\?#*{}") ||
			strings.ContainsFunc(part, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
			return "", fmt.Errorf("invalid target %q", value)
		}
	}
	id, err := resources.Parse(strings.ToLower(value))
	if err != nil {
		return "", fmt.Errorf("invalid target: %w", err)
	}
	if !id.IsResource() && !id.IsScope() {
		return "", fmt.Errorf("target is not a concrete resource or scope: %q", value)
	}
	lower := strings.ToLower(value)
	if !strings.HasPrefix(lower, "/providers/") {
		scopes := id.ScopeSegments()
		if !id.IsUCPQualified() || len(scopes) < 1 || len(scopes) > 2 ||
			scopes[0].Name == "" ||
			!containsFold([]string{"radius", "azure", "aws"}, scopes[0].Type) ||
			len(scopes) == 2 && (!strings.EqualFold(scopes[1].Type, "resourcegroups") || scopes[1].Name == "" || !strings.EqualFold(scopes[0].Type, "radius")) {
			return "", fmt.Errorf("unsupported Radius target %q", value)
		}
	}
	return lower, nil
}

func containsFold(values []string, value string) bool {
	for _, candidate := range values {
		if strings.EqualFold(candidate, value) {
			return true
		}
	}
	return false
}

func within(parent, target string) bool {
	return parent == "/" || parent == target || strings.HasPrefix(target, parent+"/")
}

// ScopeContains checks resource-ID-path ancestry, never a resource reference.
// Both IDs must be concrete, supported Radius targets.
func ScopeContains(scope, target string) (bool, error) {
	parent, err := canonicalTarget(scope)
	if err != nil {
		return false, err
	}
	child, err := canonicalTarget(target)
	if err != nil {
		return false, err
	}
	return within(parent, child), nil
}

// ValidateScope accepts installation, plane, resource group and the five
// resource types that the user-facing design permits as assignment scopes.
func ValidateScope(scope string) error {
	canonical, err := canonicalTarget(scope)
	if err != nil {
		return err
	}
	if canonical == "/" {
		return nil
	}
	id, err := resources.Parse(canonical)
	if err != nil {
		return err
	}
	if id.IsScope() {
		return nil
	}
	if !containsFold([]string{
		"Radius.Core/environments", "Radius.Core/recipePacks", "Radius.Core/applications",
		"Applications.Core/environments", "Applications.Core/applications",
	}, id.Type()) || id.IsExtensionResource() {
		return fmt.Errorf("ScopeNotAssignable: %q", scope)
	}
	return nil
}

func extensionScope(id, typ string) (string, error) {
	canonical, err := canonicalTarget(id)
	if err != nil {
		return "", err
	}
	suffix := "/providers/radius.core/" + strings.ToLower(typ) + "/"
	index := strings.LastIndex(canonical, suffix)
	if index < 0 || strings.Contains(canonical[index+len(suffix):], "/") {
		return "", fmt.Errorf("invalid %s ID %q", typ, id)
	}
	scope := canonical[:index]
	if scope == "" {
		scope = "/"
	}
	if err := ValidateScope(scope); err != nil {
		return "", err
	}
	return scope, nil
}
