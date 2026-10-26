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
// It is intentionally not installed as middleware.
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
	parts := strings.Split(strings.Trim(path, "/"), "/")
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
		// Cloud proxy operations use the plane's credential even for reads.
		// Never catalog individual cloud resource types.
		cloudProxy := strings.EqualFold(planeType, "aws") && len(parts) > 3 && strings.EqualFold(parts[3], "accounts") ||
			strings.EqualFold(planeType, "azure") && len(parts) > 3 && !strings.EqualFold(parts[3], "providers")
		if cloudProxy {
			if strings.EqualFold(planeType, "aws") &&
				(len(parts) < 10 || !strings.EqualFold(parts[5], "regions") || !strings.EqualFold(parts[7], "providers")) {
				return RequestMapping{}, fmt.Errorf("invalid AWS resource request path")
			}
			if strings.EqualFold(planeType, "azure") &&
				(len(parts) < 5 || !strings.EqualFold(parts[3], "subscriptions")) {
				return RequestMapping{}, fmt.Errorf("invalid Azure resource request path")
			}
			return c.mapping(ResourceRequest, "System."+planeType+"/credentials/use/action", plane)
		}
		// UCP-owned namespaces keep their resource permissions on cloud planes.
		if strings.EqualFold(planeType, "azure") && len(parts) > 5 && strings.EqualFold(parts[3], "providers") &&
			!containsFold([]string{"System.Azure", "System.Resources", "Radius.Core"}, parts[4]) {
			return c.mapping(ResourceRequest, "System.Azure/credentials/use/action", plane)
		}
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
		if len(parts) == 11 && strings.EqualFold(parts[9], "icons") && method == http.MethodGet {
			return c.mapping(ResourceRequest, "System.Resources/resourceProviders/resourceTypes/read", "/"+strings.Join(parts[:9], "/"))
		}
	}
	op := map[string]string{http.MethodGet: "read", http.MethodPut: "write", http.MethodPatch: "write", http.MethodDelete: "delete"}[method]
	target := path
	if method == http.MethodPost {
		index := strings.LastIndex(path, "/")
		action := path[index+1:]
		if !actionPattern.MatchString(action) {
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
		if strings.HasSuffix(strings.ToLower(target), "/providers/"+strings.ToLower(id.ProviderNamespace())) {
			target = target[:strings.LastIndex(strings.ToLower(target), "/providers/")]
		}
		if target == "" {
			target = "/"
		}
	}
	return c.mapping(kind, typ+"/"+op, target)
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
