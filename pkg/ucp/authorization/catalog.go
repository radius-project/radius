// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

// Package authorization implements pure user-facing Radius RBAC decisions.
// It does not authenticate callers, load policy, or enforce HTTP requests.
// Internal component authorization remains a separate trust boundary.
package authorization

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/radius-project/radius/pkg/ucp/authorization/internal/cataloggen"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

//go:embed catalog_generated.json
var releaseCatalog []byte

const (
	// ApplicationResources is the opt-in permission set for application developers.
	ApplicationResources = "applicationResources"
	// JoinPermissionSet is required to set or change a registration's permissionSets.
	JoinPermissionSet = "System.Resources/resourceProviders/resourceTypes/joinPermissionSet/action"
)

var resourceTypePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9.]*[.][A-Za-z0-9.]+/[A-Za-z][A-Za-z0-9]*(/[A-Za-z][A-Za-z0-9]*)*$`)
var actionPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)

// Registration is a trusted resource type registration, not a caller assertion.
// Actions enter the catalog but never enter applicationResources or custom roles.
type Registration struct {
	Type           string
	Actions        []string
	PermissionSets []string
}

// Catalog is an immutable release catalog plus a snapshot of registered types.
type Catalog struct {
	permissions map[string]string
	application map[string]bool
	registered  map[string]bool
	roles       map[string][]string
}

// NewCatalog validates registrations and creates a catalog snapshot.
func NewCatalog(registrations []Registration) (*Catalog, error) {
	var generated cataloggen.Catalog
	if err := json.Unmarshal(releaseCatalog, &generated); err != nil {
		return nil, fmt.Errorf("decode release catalog: %w", err)
	}
	catalog := &Catalog{
		permissions: map[string]string{},
		application: map[string]bool{},
		registered:  map[string]bool{},
		roles:       generated.Roles,
	}
	for _, permission := range generated.Permissions {
		catalog.permissions[strings.ToLower(permission)] = permission
	}
	for _, registration := range registrations {
		typ := strings.ToLower(registration.Type)
		if !resourceTypePattern.MatchString(registration.Type) ||
			strings.HasPrefix(typ, "system.") || strings.HasPrefix(typ, "microsoft.") ||
			strings.HasPrefix(typ, "aws.") || strings.HasPrefix(typ, "radius.core/") ||
			strings.HasPrefix(typ, "applications.core/") {
			return nil, fmt.Errorf("invalid registered resource type %q", registration.Type)
		}
		if catalog.registered[typ] {
			return nil, fmt.Errorf("duplicate registration %q", registration.Type)
		}
		catalog.registered[typ] = true
		for _, set := range registration.PermissionSets {
			if set != ApplicationResources {
				return nil, fmt.Errorf("unknown permission set %q", set)
			}
			catalog.application[typ] = true
		}
		for _, op := range []string{"read", "write", "delete", "connect/action"} {
			permission := registration.Type + "/" + op
			catalog.permissions[strings.ToLower(permission)] = permission
		}
		for _, action := range registration.Actions {
			if !actionPattern.MatchString(action) {
				return nil, fmt.Errorf("invalid registered action %q", action)
			}
			permission := registration.Type + "/" + action + "/action"
			catalog.permissions[strings.ToLower(permission)] = permission
		}
	}
	return catalog, nil
}

// Permissions returns a sorted copy, suitable for catalog inspection.
func (c *Catalog) Permissions() []string {
	result := make([]string, 0, len(c.permissions))
	for _, permission := range c.permissions {
		result = append(result, permission)
	}
	slices.Sort(result)
	return result
}

// ValidatePermissions rejects unknown permissions and wildcards.
func (c *Catalog) ValidatePermissions(permissions []string) error {
	if len(permissions) == 0 {
		return fmt.Errorf("permissions must not be empty")
	}
	for _, permission := range permissions {
		if _, ok := c.permissions[strings.ToLower(permission)]; !ok || strings.ContainsAny(permission, "*?") {
			return fmt.Errorf("UnknownPermission: %q", permission)
		}
	}
	return nil
}

// PermissionSetChecks returns the additional registration permission when
// permissionSets is supplied on create, or changed on update. The normal
// registration write permission is independently required.
func PermissionSetChecks(target string, before, after []string, creating bool) ([]Check, error) {
	scope, err := canonicalTarget(target)
	if err != nil {
		return nil, err
	}
	id, err := resources.Parse(scope)
	if err != nil || !strings.EqualFold(id.Type(), "System.Resources/resourceProviders/resourceTypes") ||
		strings.Contains(scope, "/resourcegroups/") {
		return nil, fmt.Errorf("permissionSets require a plane-scoped resource type registration")
	}
	for _, set := range after {
		if set != ApplicationResources {
			return nil, fmt.Errorf("unknown permission set %q", set)
		}
	}
	oldSets, newSets := slices.Clone(before), slices.Clone(after)
	slices.Sort(oldSets)
	slices.Sort(newSets)
	if creating && after != nil || !creating && !slices.Equal(slices.Compact(oldSets), slices.Compact(newSets)) {
		return []Check{{Permission: JoinPermissionSet, Target: scope}}, nil
	}
	return nil, nil
}
