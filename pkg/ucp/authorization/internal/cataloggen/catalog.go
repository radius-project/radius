// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

// Package cataloggen generates the release permission catalog from checked-in APIs.
package cataloggen

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Route describes a Swagger operation, not necessarily a live UCP endpoint.
type Route struct {
	Method     string `json:"method"`
	Path       string `json:"path"`
	Permission string `json:"permission"`
}

// Catalog is the deterministic build artifact.
type Catalog struct {
	Permissions []string            `json:"permissions"`
	Routes      []Route             `json:"routes"`
	Roles       map[string][]string `json:"roles"`
}

// OwnedPermissions is the explicit UCP-owned permission list. UCP's Swagger
// includes PATCH operations that its current routers do not serve.
func OwnedPermissions() []string {
	var result []string
	for _, typ := range []string{
		"System.Radius/planes", "System.Azure/planes", "System.AWS/planes",
		"System.Resources/resourceGroups", "System.Resources/resourceProviders",
		"System.Resources/resourceProviders/locations",
		"System.Resources/resourceProviders/resourceTypes",
		"System.Resources/resourceProviders/resourceTypes/apiVersions",
		"System.Azure/credentials", "System.AWS/credentials",
		"Radius.Core/roleDefinitions", "Radius.Core/roleAssignments",
	} {
		for _, op := range []string{"read", "write", "delete"} {
			result = append(result, typ+"/"+op)
		}
	}
	return append(result,
		"System.Resources/planes/read",
		"System.Resources/resources/read",
		"System.Resources/providers/read",
		"System.Resources/resourceProviders/resourceTypes/joinPermissionSet/action",
		"System.Azure/credentials/use/action", "System.AWS/credentials/use/action",
		"Radius.Core/authorizationSettings/read", "Radius.Core/authorizationSettings/write",
		"Radius.Core/permissions/read", "Radius.Core/accessChanges/read",
		"Radius.Core/checkAccess/action", "Radius.Core/explainAccess/action",
		"Radius.Core/effectiveAccess/read",
	)
}

// Generate reads the Swagger and registered built-in manifests beneath root.
// It never contacts a registry or modifies its inputs.
func Generate(root string) (Catalog, error) {
	result := Catalog{Permissions: OwnedPermissions(), Roles: ReleaseRoles()}
	err := filepath.WalkDir(filepath.Join(root, "swagger/specification"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "openapi.json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var spec struct {
			Paths map[string]map[string]json.RawMessage `json:"paths"`
		}
		if err := json.Unmarshal(data, &spec); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		for path, operations := range spec.Paths {
			for method := range operations {
				if !slices.Contains([]string{"get", "put", "patch", "delete", "post"}, method) {
					continue
				}
				permission, ok := swaggerPermission(path, method)
				if !ok {
					continue
				}
				result.Permissions = append(result.Permissions, permission)
				result.Routes = append(result.Routes, Route{strings.ToUpper(method), path, permission})
			}
		}
		return nil
	})
	if err != nil {
		return Catalog{}, err
	}
	manifests, err := filepath.Glob(filepath.Join(root, "deploy/manifest/built-in-providers/self-hosted/*.yaml"))
	if err != nil {
		return Catalog{}, err
	}
	for _, path := range manifests {
		data, err := os.ReadFile(path)
		if err != nil {
			return Catalog{}, err
		}
		var manifest struct {
			Namespace string         `yaml:"namespace"`
			Types     map[string]any `yaml:"types"`
		}
		if err := yaml.Unmarshal(data, &manifest); err != nil {
			return Catalog{}, fmt.Errorf("%s: %w", path, err)
		}
		for typ := range manifest.Types {
			if !strings.HasPrefix(manifest.Namespace, "Radius.") && manifest.Namespace != "Microsoft.Resources" {
				continue
			}
			for _, op := range []string{"read", "write", "delete"} {
				result.Permissions = append(result.Permissions, manifest.Namespace+"/"+typ+"/"+op)
			}
		}
	}
	// Use/connect are semantic permissions, not HTTP CRUD operations.
	types := map[string]bool{}
	for _, permission := range result.Permissions {
		if strings.HasSuffix(permission, "/write") {
			types[strings.TrimSuffix(permission, "/write")] = true
		}
	}
	for typ := range types {
		if strings.HasPrefix(typ, "Applications.") || strings.HasPrefix(typ, "Radius.") && !strings.HasPrefix(typ, "Radius.Core/") ||
			slices.Contains([]string{"Radius.Core/applications"}, typ) {
			result.Permissions = append(result.Permissions, typ+"/connect/action")
		}
	}
	result.Permissions = append(result.Permissions,
		"Radius.Core/environments/use/action", "Applications.Core/environments/use/action",
		"Radius.Core/recipePacks/use/action")
	slices.Sort(result.Permissions)
	result.Permissions = slices.Compact(result.Permissions)
	slices.SortFunc(result.Routes, func(a, b Route) int {
		return strings.Compare(a.Path+" "+a.Method, b.Path+" "+b.Method)
	})
	result.Routes = slices.Compact(result.Routes)
	return result, nil
}

func swaggerPermission(path, method string) (string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	provider := slices.Index(parts, "providers")
	if provider < 0 || provider+2 >= len(parts) {
		return "", false
	}
	namespace := parts[provider+1]
	if namespace != "Radius.Core" && !strings.HasPrefix(namespace, "Applications.") {
		return "", false
	}
	tail := parts[provider+2:]
	// These provider-level operations are discovery/self-inspection, not resources.
	if len(tail) == 1 && slices.Contains([]string{"operations", "whoAmI", "checkAccess", "explainAccess", "effectiveAccess"}, tail[0]) {
		switch tail[0] {
		case "checkAccess", "explainAccess":
			return namespace + "/" + tail[0] + "/action", true
		case "effectiveAccess":
			return namespace + "/effectiveAccess/read", true
		default:
			return "", false
		}
	}
	typ := namespace
	for i := 0; i < len(tail); i += 2 {
		typ += "/" + tail[i]
	}
	if method == "post" {
		// The final segment is an action, not a child resource type.
		if len(tail) < 3 || len(tail)%2 != 1 {
			return "", false
		}
		return typ + "/action", true
	}
	op := map[string]string{"get": "read", "put": "write", "patch": "write", "delete": "delete"}[method]
	return typ + "/" + op, true
}
