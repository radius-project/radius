// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// ObserveRequests records DryRun target decisions without modifying workload
// requests or responses. Authentication must run first. Reserved policy mutations
// are rejected even with no runtime: no public safe policy/settings writer is
// installed in this slice. These guards run before generic routing and proxies.
func ObserveRequests(runtime *PolicyRuntime, pathBase string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if pathBase != "" && (path == pathBase || strings.HasPrefix(path, pathBase+"/")) {
				path = strings.TrimPrefix(path, pathBase)
				if path == "" {
					path = "/"
				}
			}
			identity, verified := IdentityFromContext(r.Context())
			logger := ucplog.FromContextOrDiscard(r.Context())
			if protectedMutation(r.Method, path) {
				logger.Info("User authorization request", "result", "deny", "reason", "protectedMutationUnsupported")
				if !verified {
					if err := rest.NewClientAuthenticationFailedARMResponse().Apply(r.Context(), w, r); err != nil {
						logger.Error(err, "Failed to write authentication error response")
					}
					return
				}
				writeAuthorizationUnavailable(w, r)
				return
			}
			if operation, _ := inspectionRoute(path); operation != "" {
				// These operations perform explicit authentication and policy
				// checks in Inspection, including in unsupported workload Enforce.
				next.ServeHTTP(w, r)
				return
			}
			if runtime == nil {
				next.ServeHTTP(w, r)
				return
			}
			result, err := runtime.EvaluateRequest(identity, r.Method, path)
			if result.Mode == api.UserAuthorizationModeEnforce && result.Mapping.Kind != DiscoveryRequest {
				logger.Info("User authorization request", "result", "deny", "reason", "enforceUnsupported")
				writeAuthorizationUnavailable(w, r)
				return
			}
			if result.Mode == api.UserAuthorizationModeDisabled {
				next.ServeHTTP(w, r)
				return
			}
			fields := []any{
				"mode", string(result.Mode), "generation", result.Generation, "coverage", "targetOnly",
				"method", r.Method, "kind", result.Mapping.Kind, "authenticated", verified,
			}
			if verified {
				fields = append(fields, "principal", identity.Principal)
			}
			if err != nil {
				// Do not label an unavailable/indeterminate check as an allow or
				// wouldDeny. In particular LastKnownMode is not a current mode.
				logger.Error(err, "User authorization request", append(fields, "result", "indeterminate")...)
			} else if result.Mapping.Kind != DiscoveryRequest {
				for _, decision := range result.Decisions {
					outcome := "allow"
					if !decision.Allowed {
						outcome = "wouldDeny"
					}
					target := decision.Target
					if protectedTarget(target) {
						target = "[redacted]"
					}
					logger.Info("User authorization request", append(fields,
						"result", outcome, "permission", decision.Permission, "target", target)...)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeAuthorizationUnavailable(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	if err := json.NewEncoder(w).Encode(v1.ErrorResponse{Error: &v1.ErrorDetails{
		Code: "AuthorizationUnavailable", Message: "This authorization operation is unavailable.",
	}}); err != nil {
		ucplog.FromContextOrDiscard(r.Context()).Error(err, "Failed to write authorization error response")
	}
}

func protectedMutation(method, path string) bool {
	if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
		return false
	}
	return protectedTarget(path)
}

func protectedTarget(path string) bool {
	// Match Azure proxy reparsing, including query/fragment delimiters, rather
	// than just unescaping. Inspect successive targets with a bounded depth.
	for depth := 0; ; depth++ {
		if protectedPath(path) {
			return true
		}
		target, err := url.Parse(path)
		if err != nil {
			// A parse failure has no further routing target. In particular a
			// literal percent produced by an earlier parse is not a policy
			// mutation. Actual invalid requests remain the router's concern.
			return false
		}
		// This is a fixed-point check, not a trust comparison: the identical
		// target was already checked against the protected namespaces above.
		if target.Path == path {
			return false
		}
		if depth == 8 {
			return true
		}
		path = target.Path
	}
}

func protectedPath(path string) bool {
	// Match namespace/type pairs anywhere in the path, including extension
	// resources and proxy paths. Do not rely on successful catalog mapping:
	// malformed, unknown, and private store targets must not bypass the guard.
	parts := strings.FieldsFunc(strings.ToLower(path), func(r rune) bool { return r == '/' })
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] != "providers" {
			continue
		}
		if parts[i+1] == "system.authorization" {
			return true
		}
		if i+2 < len(parts) && parts[i+1] == "radius.core" &&
			containsFold([]string{"roleDefinitions", "roleAssignments", "authorizationSettings", "accessChanges"}, parts[i+2]) {
			return true
		}
	}
	return false
}
