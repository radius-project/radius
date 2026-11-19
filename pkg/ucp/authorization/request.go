// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/radius-project/radius/pkg/ucp/resources"
)

// RequestKind distinguishes checks needing later response or trusted-context
// handling. No kind is an authorization grant by itself.
type RequestKind string

const (
	// ResourceRequest requires all mapped permission checks.
	ResourceRequest RequestKind = "resource"
	// ListRequest requires parent read or per-item filtering, not a blanket deny.
	ListRequest RequestKind = "list"
	// MixedResourceListRequest always requires per-item checks using each item's
	// real type. Generic resources/read cannot grant role or credential contents.
	MixedResourceListRequest RequestKind = "mixedResourceList"
	// DiscoveryRequest deliberately has no resource-content permission.
	DiscoveryRequest RequestKind = "discovery"
	// SelfAccessRequest requires a signed-in identity; checking someone else
	// additionally requires the mapped permission.
	SelfAccessRequest RequestKind = "selfAccess"
	// OperationStatusRequest requires resolving the operation's original target
	// and permission, or matching its verified initiating principal.
	OperationStatusRequest RequestKind = "operationStatus"
)

// RequestMapping describes required authorization work without performing it.
type RequestMapping struct {
	Kind   RequestKind
	Checks []Check
}

// MapRequest maps a normalized path after the configured UCP path base has
// been removed. It rejects unknown actions, malformed IDs and unknown types.
// It does not authenticate, enforce, inspect references, or filter responses.
func (c *Catalog) MapRequest(method, path string) (RequestMapping, error) {
	if !slices.Contains([]string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodPost}, method) {
		return RequestMapping{}, fmt.Errorf("unsupported request method %q", method)
	}
	if strings.Contains(path, "//") {
		return RequestMapping{}, fmt.Errorf("invalid request path %q", path)
	}
	// Permit a trailing slash as chi's collection routes do, but never clean
	// duplicate slashes, dot segments, escaping or other ambiguous paths.
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		path = "/"
	}
	if err := validateRequestPath(path); err != nil {
		return RequestMapping{}, err
	}
	lower := strings.ToLower(path)
	if method == http.MethodGet && slices.Contains([]string{"/", "/version", "/openapi/v2", "/openapi/v3"}, lower) {
		return RequestMapping{Kind: DiscoveryRequest}, nil
	}
	if method == http.MethodGet && lower == "/providers/radius.core/whoami" {
		return RequestMapping{Kind: SelfAccessRequest}, nil
	}
	for _, namespace := range []string{"radius.core", "applications.core", "applications.dapr", "applications.datastores", "applications.messaging"} {
		if method == http.MethodGet && lower == "/providers/"+namespace+"/operations" {
			return RequestMapping{Kind: DiscoveryRequest}, nil
		}
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	// Classify cloud plane routes before interpreting a suffix as an inspection
	// API. chi matches fixed segments exactly, so these checks must not fold case.
	if len(parts) >= 4 && strings.EqualFold(parts[0], "planes") {
		plane := "/" + strings.Join(parts[:3], "/")
		below := parts[3:]
		if strings.EqualFold(parts[1], "azure") {
			// Azure serves only the literal credential routes locally; every
			// other path below the plane is proxied with the plane's credential.
			if len(below) < 3 || below[0] != "providers" || below[1] != "System.Azure" || below[2] != "credentials" {
				// planes.ProxyController rejects paths without a provider
				// resource before forwarding, so no credential is used.
				if id, err := resources.Parse(path); err != nil || id.ProviderNamespace() == "" {
					return RequestMapping{}, fmt.Errorf("Azure proxy route requires a provider resource")
				}
				return c.mapping(ResourceRequest, "System.Azure/credentials/use/action", plane)
			}
			if !(len(below) == 3 && method == http.MethodGet ||
				len(below) == 4 && slices.Contains([]string{http.MethodGet, http.MethodPut, http.MethodDelete}, method)) {
				return RequestMapping{}, fmt.Errorf("unsupported Azure credential route")
			}
		}
		if strings.EqualFold(parts[1], "aws") && strings.EqualFold(below[0], "accounts") {
			kind, ok := awsProxyRoute(method, below)
			if !ok {
				return RequestMapping{}, fmt.Errorf("unsupported AWS resource route")
			}
			if kind == OperationStatusRequest {
				return RequestMapping{Kind: kind}, nil
			}
			return c.mapping(kind, "System.AWS/credentials/use/action", plane)
		}
	}
	for _, action := range []string{"checkaccess", "explainaccess", "effectiveaccess"} {
		suffix := "/providers/radius.core/" + action
		if strings.HasSuffix(lower, suffix) {
			target := path[:len(path)-len(suffix)]
			if target == "" {
				target = "/"
			}
			if err := ValidateScope(target); err != nil {
				return RequestMapping{}, err
			}
			if action == "effectiveaccess" && method == http.MethodGet {
				return c.mapping(ResourceRequest, "Radius.Core/roleAssignments/read", target)
			}
			if action != "effectiveaccess" && method == http.MethodPost {
				return c.mapping(SelfAccessRequest, "Radius.Core/"+action+"/action", target)
			}
			return RequestMapping{}, fmt.Errorf("unsupported access inspection method")
		}
	}
	if method == http.MethodGet && lower == "/planes" {
		return c.mapping(ListRequest, "System.Resources/planes/read", "/")
	}
	if len(parts) >= 2 && strings.EqualFold(parts[0], "planes") && containsFold([]string{"radius", "aws", "azure"}, parts[1]) {
		planeType := parts[1]
		if len(parts) == 2 {
			if method != http.MethodGet {
				return RequestMapping{}, fmt.Errorf("unsupported plane collection method")
			}
			return c.mapping(ListRequest, "System."+planeType+"/planes/read", "/")
		}
		plane := "/" + strings.Join(parts[:3], "/")
		if method == http.MethodGet && len(parts) >= 8 &&
			strings.EqualFold(parts[len(parts)-4], "locations") &&
			containsFold([]string{"operationStatuses", "operationResults"}, parts[len(parts)-2]) {
			return RequestMapping{Kind: OperationStatusRequest}, nil
		}
		if len(parts) == 4 && strings.EqualFold(parts[3], "providers") && method == http.MethodGet {
			return c.mapping(ListRequest, "System.Resources/providers/read", plane)
		}
		if len(parts) == 5 && strings.EqualFold(parts[3], "providers") && method == http.MethodGet {
			return c.mapping(ResourceRequest, "System.Resources/providers/read", plane)
		}
		if len(parts) == 4 && strings.EqualFold(parts[3], "resourcegroups") && method == http.MethodGet {
			return c.mapping(ListRequest, "System.Resources/resourceGroups/read", plane)
		}
		if len(parts) == 6 && strings.EqualFold(parts[3], "resourcegroups") && strings.EqualFold(parts[5], "resources") && method == http.MethodGet {
			return c.mapping(MixedResourceListRequest, "System.Resources/resources/read", "/"+strings.Join(parts[:5], "/"))
		}
		// Match the icon route's fixed segments as routed; only the
		// authorization target is folded.
		if len(parts) == 11 && strings.EqualFold(planeType, "radius") &&
			parts[3] == "providers" && parts[4] == "System.Resources" && parts[5] == "resourceproviders" &&
			parts[7] == "resourcetypes" && parts[9] == "icons" && method == http.MethodGet {
			return c.mapping(ResourceRequest, "System.Resources/resourceProviders/resourceTypes/read", "/"+strings.Join(parts[:9], "/"))
		}
	}
	// Only deployment operation collection/item routes read the parent.
	// Extension resources must retain their own permission and target.
	if method == http.MethodGet {
		for i := 0; i+4 < len(parts); i++ {
			if strings.EqualFold(parts[i], "providers") && strings.EqualFold(parts[i+1], "Microsoft.Resources") &&
				strings.EqualFold(parts[i+2], "deployments") &&
				((len(parts) == i+5 && strings.EqualFold(parts[i+4], "operations")) ||
					(len(parts) == i+6 && containsFold([]string{"operations", "operationStatuses"}, parts[i+4]))) {
				return c.mapping(ResourceRequest, "Microsoft.Resources/deployments/read", "/"+strings.Join(parts[:i+4], "/"))
			}
		}
	}
	op := map[string]string{http.MethodGet: "read", http.MethodPut: "write", http.MethodPatch: "write", http.MethodDelete: "delete"}[method]
	target := path
	if method == http.MethodPost {
		index := strings.LastIndex(path, "/")
		action := path[index+1:]
		// POST invokes a callable action; it never grants semantic permissions.
		if !actionPattern.MatchString(action) || isReservedAction(action) {
			return RequestMapping{}, fmt.Errorf("invalid request action")
		}
		target = path[:index]
		op = action + "/action"
	}
	id, err := resources.Parse(strings.ToLower(target))
	if err != nil {
		return RequestMapping{}, err
	}
	typ := id.Type()
	kind := ResourceRequest
	if id.IsResourceCollection() {
		if method != http.MethodGet {
			return RequestMapping{}, fmt.Errorf("unsupported collection request")
		}
		kind = ListRequest
		typ = id.Type()
		target = target[:strings.LastIndex(target, "/")]
		// Removing a collection's type leaves /providers/namespace, not its
		// containing scope. Nested child collections retain their parent resource.
		parent := strings.ToLower(target)
		if strings.HasSuffix(parent, "/providers/"+strings.ToLower(id.ProviderNamespace())) {
			// Unicode case conversion can change byte lengths. The offset and
			// the sliced string must use the same representation.
			target = parent[:strings.LastIndex(parent, "/providers/")]
		}
		if target == "" {
			target = "/"
		}
	}
	return c.mapping(kind, typ+"/"+op, target)
}

// normalizedRoutePath applies middleware.NormalizePath's rewrite, which the
// frontend performs before dispatch, so the route gate sees what chi routes.
func normalizedRoutePath(path string) string {
	for _, segment := range []string{"/planes", "/resourcegroups"} {
		if index := strings.Index(strings.ToLower(path), segment); index >= 0 {
			path = path[:index] + segment + path[index+len(segment):]
		}
	}
	return path
}

// ServedRoute reports whether UCP's frontend dispatcher has a handler for method
// and path, relative to the path base, among the routes MapRequest maps to
// checks. It mirrors api.Register and the radius, aws and azure modules: after
// NormalizePath's rewrite, the plane module key, every other fixed segment,
// registered methods and leaf trailing slashes must match exactly. Proxied
// subtrees are accepted for any method; MapRequest checks their structure.
// The catalog also maps API routes this frontend does not mount, so DryRun
// records unserved requests as indeterminate.
func ServedRoute(method, path string) bool {
	trailingSlash := path != "/" && strings.HasSuffix(path, "/")
	return servedRoute(method, strings.Split(strings.Trim(normalizedRoutePath(path), "/"), "/"), trailingSlash)
}

func servedRoute(method string, parts []string, trailingSlash bool) bool {
	get := method == http.MethodGet
	crud := slices.Contains([]string{http.MethodGet, http.MethodPut, http.MethodDelete}, method)
	if len(parts) == 0 || parts[0] != "planes" {
		return false
	}
	if len(parts) == 1 || len(parts) == 2 {
		return get && (len(parts) == 1 || slices.Contains([]string{"radius", "aws", "azure"}, parts[1]))
	}
	if !slices.Contains([]string{"radius", "aws", "azure"}, parts[1]) {
		return false
	}
	if len(parts) == 3 {
		return crud
	}
	below := parts[3:]
	switch parts[1] {
	case "azure":
		// Local credential routes and the catch-all proxy are classified by MapRequest.
		return true
	case "aws":
		if below[0] == "accounts" {
			// Only resource collections accept a trailing slash.
			return !trailingSlash || len(below) == 7
		}
		return len(below) >= 3 && below[0] == "providers" && below[1] == "System.AWS" && below[2] == "credentials" &&
			(len(below) == 3 && get || len(below) == 4 && crud)
	}
	switch below[0] {
	case "providers":
		if len(below) == 1 {
			return get
		}
		if below[1] != "System.Resources" {
			// /providers/{name} is a summary leaf; deeper paths are proxied.
			return len(below) > 2 || get && !trailingSlash
		}
		return radiusSystemResourcesRoute(method, below[2:], trailingSlash)
	case "resourcegroups":
		switch {
		case len(below) == 1:
			return get
		case len(below) == 2:
			return crud
		case len(below) == 3 && below[2] == "resources":
			return get
		}
		return len(below) > 3 && below[2] == "providers"
	}
	return false
}

// radiusSystemResourcesRoute matches the routes below
// /planes/radius/{plane}/providers/System.Resources.
func radiusSystemResourcesRoute(method string, rest []string, trailingSlash bool) bool {
	get := method == http.MethodGet
	crud := slices.Contains([]string{http.MethodGet, http.MethodPut, http.MethodDelete}, method)
	if len(rest) == 4 && rest[0] == "locations" && (rest[2] == "operationStatuses" || rest[2] == "operationResults") {
		return get && !trailingSlash
	}
	if len(rest) == 0 || rest[0] != "resourceproviders" {
		return false
	}
	switch len(rest) {
	case 1:
		return get
	case 2:
		return crud
	}
	switch rest[2] {
	case "locations":
		return len(rest) == 3 && get || len(rest) == 4 && crud
	case "resourcetypes":
	default:
		return false
	}
	switch {
	case len(rest) == 3:
		return get
	case len(rest) == 4:
		return crud
	case len(rest) == 5 && rest[4] == "apiversions":
		return get
	case len(rest) == 6 && rest[4] == "apiversions":
		return crud
	case len(rest) == 6 && rest[4] == "icons":
		return get && !trailingSlash
	}
	return false
}

// awsProxyRoute reports whether below (the path after /planes/aws/{plane})
// is one of the finite AWS resource routes in frontend/aws/routes.go.
func awsProxyRoute(method string, below []string) (RequestKind, bool) {
	if len(below) < 7 || below[0] != "accounts" || below[2] != "regions" || below[4] != "providers" {
		return "", false
	}
	switch len(below) {
	case 7:
		return ResourceRequest, method == http.MethodGet
	case 8:
		if below[7] == ":get" {
			return ResourceRequest, method == http.MethodPost
		}
		return ResourceRequest, slices.Contains([]string{http.MethodGet, http.MethodPut, http.MethodDelete}, method)
	case 10:
		return OperationStatusRequest, method == http.MethodGet && below[6] == "locations" &&
			(below[8] == "operationResults" || below[8] == "operationStatuses")
	}
	return "", false
}

func validateRequestPath(path string) error {
	if path == "/" {
		return nil
	}
	if !utf8.ValidString(path) {
		return fmt.Errorf("request path must be valid UTF-8")
	}
	for _, segment := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if segment == "" || segment == "." || segment == ".." || strings.ContainsAny(segment, "%\\?#*{}") ||
			strings.ContainsFunc(segment, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
			return fmt.Errorf("invalid request path %q", path)
		}
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("request path must be absolute")
	}
	return nil
}

func (c *Catalog) mapping(kind RequestKind, permission, target string) (RequestMapping, error) {
	if err := c.ValidatePermissions([]string{permission}); err != nil {
		return RequestMapping{}, err
	}
	canonical, err := canonicalTarget(target)
	if err != nil {
		return RequestMapping{}, err
	}
	return RequestMapping{Kind: kind, Checks: []Check{{c.permissions[strings.ToLower(permission)], canonical}}}, nil
}
