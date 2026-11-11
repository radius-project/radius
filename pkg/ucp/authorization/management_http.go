// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/google/uuid"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

var managementName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

func managementRoute(path string) (managementTarget, bool, error) {
	lower := strings.ToLower(path)
	const prefix = "/providers/radius.core/"
	index := strings.LastIndex(lower, prefix)
	if index < 0 {
		return managementTarget{}, false, nil
	}
	parts := strings.Split(lower[index+len(prefix):], "/")
	kind := ""
	switch parts[0] {
	case "roledefinitions":
		kind = "roleDefinitions"
	case "roleassignments":
		kind = "roleAssignments"
	case "accesschanges":
		kind = "accessChanges"
	default:
		return managementTarget{}, false, nil
	}
	// Unknown actions and malformed suffixes remain behind the generic guard.
	if len(parts) > 2 || len(parts) == 2 && !managementName.MatchString(parts[1]) {
		return managementTarget{}, false, nil
	}
	scope := lower[:index]
	if scope == "" {
		scope = "/"
	}
	target := managementTarget{id: lower, scope: scope, kind: kind}
	if len(parts) == 2 {
		target.name = parts[1]
	}
	return target, true, validateManagementScope(scope, kind)
}

// Management serves role, assignment and read-only history APIs before generic routing.
// Authentication must run first. Other policy mutations remain guarded by
// ObserveRequests, including settings, history and private store targets.
func Management(runtime *PolicyRuntime, pathBase string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if pathBase != "" {
				if !strings.HasPrefix(path, pathBase+"/") {
					next.ServeHTTP(w, r)
					return
				}
				path = strings.TrimPrefix(path, pathBase)
			}
			target, matched, routeErr := managementRoute(path)
			if !matched {
				next.ServeHTTP(w, r)
				return
			}
			if target.kind == "accessChanges" {
				if r.Method != http.MethodGet {
					next.ServeHTTP(w, r)
					return
				}
				serveHistory(w, r, runtime, target, routeErr)
				return
			}
			identity, verified := IdentityFromContext(r.Context())
			outcome, auditRequestID := "deny", ""
			defer func() {
				fields := []any{"result", outcome, "reason", "policyManagement", "requestId", auditRequestID,
					"authenticated", verified, "method", r.Method}
				if verified {
					fields = append(fields, "principal", identity.Principal)
				}
				ucplog.FromContextOrDiscard(r.Context()).Info("User authorization request", fields...)
			}()
			if !verified {
				writeInspectionError(w, r, http.StatusUnauthorized, "InvalidAuthenticationInfo", "A verified Kubernetes caller identity is required.")
				return
			}
			if err := runtime.Ready(); err != nil {
				writeAuthorizationUnavailable(w, r)
				return
			}
			if routeErr != nil {
				writeManagementError(w, r, &policyMutationError{routeErr})
				return
			}
			allow := "GET, PUT, DELETE"
			if target.name == "" {
				allow = "GET"
			}
			if r.Method != http.MethodGet && (target.name == "" || r.Method != http.MethodPut && r.Method != http.MethodDelete) {
				w.Header().Set("Allow", allow)
				writeInspectionError(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "This policy operation allows "+allow+".")
				return
			}
			query, err := url.ParseQuery(r.URL.RawQuery)
			if err != nil || len(query["api-version"]) != 1 || query.Get("api-version") != "2025-08-01-preview" {
				writeInspectionError(w, r, http.StatusBadRequest, "InvalidApiVersionParameter", "Use one api-version=2025-08-01-preview.")
				return
			}
			descendants, takeover, err := managementOptions(query, target, r.Method)
			if err != nil {
				writeInspectionError(w, r, http.StatusBadRequest, "InvalidRequestContent", err.Error())
				return
			}
			requestID, err := uuid.NewRandom()
			if err != nil {
				writeManagementError(w, r, err)
				return
			}
			w.Header().Set("X-Ms-Request-Id", requestID.String())
			auditRequestID = requestID.String()
			actor := PolicyActor{
				Identity: identity, RequestID: requestID.String(), TakeOwnership: takeover,
				CorrelationID: requestID.String(),
				Management:    api.AssignmentManagement{Kind: to.Ptr(api.AssignmentManagementKindImperative)},
			}
			var result any
			if r.Method == http.MethodGet {
				result, err = runtime.readManagement(identity, target, descendants)
			} else {
				operation := "write"
				if r.Method == http.MethodDelete {
					operation = "delete"
				}
				// Reject unauthorized requests before interpreting their bodies.
				// The store repeats this check against current policy on retries.
				err = runtime.authorizeManagement(identity, target, operation)
				if err == nil {
					mutation := PolicyMutation{Target: target.id, Delete: r.Method == http.MethodDelete}
					if !mutation.Delete {
						mutation, err = decodeManagement(w, r, target)
					}
					if err == nil {
						var snapshot *PolicySnapshot
						snapshot, err = runtime.mutate(r.Context(), actor, mutation)
						if err == nil && !mutation.Delete {
							// Return this commit's value, not a second read that
							// could observe a concurrent replacement or removal.
							result, err = managementResource(snapshot, target)
						}
					}
				}
			}
			if err != nil {
				writeManagementError(w, r, err)
				return
			}
			outcome = "allow"
			if r.Method == http.MethodDelete {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(result); err != nil {
				ucplog.FromContextOrDiscard(r.Context()).Error(err, "Failed to write policy management response")
			}
		})
	}
}

func managementOptions(query url.Values, target managementTarget, method string) (descendants, takeover bool, err error) {
	for name, values := range query {
		if len(values) != 1 {
			return false, false, fmt.Errorf("query parameter %s must occur once", name)
		}
		switch {
		case name == "api-version":
		case name == "$filter" && method == http.MethodGet && target.name == "" && target.kind == "roleAssignments":
			if values[0] != "atScopeAndBelow()" {
				return false, false, fmt.Errorf("unsupported assignment filter")
			}
			descendants = true
		case name == "takeOwnership" && method == http.MethodDelete && target.kind == "roleAssignments":
			if values[0] != "true" && values[0] != "false" {
				return false, false, fmt.Errorf("takeOwnership must be true or false")
			}
			takeover = values[0] == "true"
		default:
			return false, false, fmt.Errorf("unsupported query parameter %s", name)
		}
	}
	return descendants, takeover, nil
}

func decodeManagement(w http.ResponseWriter, r *http.Request, target managementTarget) (PolicyMutation, error) {
	mutation := PolicyMutation{Target: target.id}
	// Generated unmarshallers ignore unknown fields. Plain wire structs keep
	// server-owned metadata and caller-asserted memberships out of this boundary.
	var role struct {
		Properties *struct {
			DisplayName      *string   `json:"displayName"`
			Description      *string   `json:"description"`
			Permissions      []*string `json:"permissions"`
			AssignableScopes []*string `json:"assignableScopes"`
		} `json:"properties"`
	}
	var assignment struct {
		Properties *struct {
			RoleDefinitionID *string `json:"roleDefinitionId"`
			Description      *string `json:"description"`
			Principal        *struct {
				Type        *api.PrincipalType `json:"type"`
				Issuer      *string            `json:"issuer"`
				Subject     *string            `json:"subject"`
				DisplayName *string            `json:"displayName"`
			} `json:"principal"`
		} `json:"properties"`
	}
	var wire any = &assignment
	if target.kind == "roleDefinitions" {
		wire = &role
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(wire); err != nil {
		return mutation, &policyMutationError{err}
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return mutation, &policyMutationError{errors.New("expected exactly one JSON request object")}
	}
	if target.kind == "roleDefinitions" {
		p := role.Properties
		if p == nil || len(p.Permissions) == 0 {
			return mutation, &policyMutationError{errors.New("properties.permissions must not be empty")}
		}
		for _, permission := range p.Permissions {
			if permission == nil {
				return mutation, &policyMutationError{errors.New("permission cannot be null")}
			}
			if err := validateAccessString("permission", *permission, maxPermissionLength); err != nil {
				return mutation, &policyMutationError{err}
			}
		}
		for _, scope := range p.AssignableScopes {
			if scope == nil {
				return mutation, &policyMutationError{errors.New("assignable scope cannot be null")}
			}
			if err := validateAccessString("assignableScope", *scope, maxAccessScopeLength); err != nil {
				return mutation, &policyMutationError{err}
			}
		}
		mutation.Role = &api.RoleDefinitionProperties{
			DisplayName: p.DisplayName, Description: p.Description,
			Permissions: p.Permissions, AssignableScopes: p.AssignableScopes,
		}
		return mutation, nil
	}
	p := assignment.Properties
	if p == nil || p.RoleDefinitionID == nil || p.Principal == nil {
		return mutation, &policyMutationError{errors.New("properties.roleDefinitionId and properties.principal are required")}
	}
	roleID, matched, err := managementRoute(*p.RoleDefinitionID)
	if err != nil {
		return mutation, &policyMutationError{err}
	}
	if !matched || roleID.kind != "roleDefinitions" || roleID.name == "" {
		return mutation, &policyMutationError{errors.New("roleDefinitionId must be a full role definition resource ID")}
	}
	principal := &api.Principal{Type: p.Principal.Type, Issuer: p.Principal.Issuer, Subject: p.Principal.Subject, DisplayName: p.Principal.DisplayName}
	if err := validateManagementPrincipal(principal); err != nil {
		return mutation, &policyMutationError{err}
	}
	mutation.Assignment = &api.RoleAssignmentProperties{
		RoleDefinitionID: p.RoleDefinitionID, Principal: principal, Description: p.Description,
	}
	return mutation, nil
}

func writeManagementError(w http.ResponseWriter, r *http.Request, err error) {
	var rejected *policyMutationError
	switch {
	case errors.Is(err, ErrAuthorizationUnavailable):
		ucplog.FromContextOrDiscard(r.Context()).Error(err, "Policy management unavailable; inspect policy before retrying a write")
		writeAuthorizationUnavailable(w, r)
	case errors.Is(err, ErrPolicyConflict):
		writeInspectionError(w, r, http.StatusConflict, "Conflict", "Concurrent policy changes exceeded the retry limit; read policy and retry.")
	case errors.As(err, &rejected):
		code, _, _ := strings.Cut(rejected.Error(), ":")
		status := http.StatusBadRequest
		switch code {
		case "AuthorizationFailed", "PrivilegeEscalationDenied":
			status = http.StatusForbidden
		case "LastAdministratorProtected", "ManagedByConflict", "RoleAssignmentImmutable", "BuiltInRoleImmutable", "RoleDefinitionInUse", "PolicyCapacityExceeded":
			status = http.StatusConflict
		case "UnknownPermission", "ScopeNotAssignable":
		default:
			code = "InvalidRequestContent"
		}
		var denied *policyAccessError
		if errors.As(err, &denied) {
			identity, _ := IdentityFromContext(r.Context())
			detail := &v1.ErrorDetails{Code: code, Message: rejected.Error(), Target: denied.target,
				AdditionalInfo: []*v1.ErrorAdditionalInfo{{Type: "RadiusAuthorization", Info: map[string]any{
					"principal": identity.Principal, "permission": denied.permission, "target": denied.target, "source": "Radius",
				}}},
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			if err := json.NewEncoder(w).Encode(v1.ErrorResponse{Error: &v1.ErrorDetails{
				Code: code, Message: rejected.Error(), Details: []*v1.ErrorDetails{detail},
			}}); err != nil {
				ucplog.FromContextOrDiscard(r.Context()).Error(err, "Failed to write policy authorization error")
			}
			return
		}
		writeInspectionError(w, r, status, code, rejected.Error())
	case errors.Is(err, errManagementNotFound):
		writeInspectionError(w, r, http.StatusNotFound, "ResourceNotFound", "The requested authorization resource does not exist.")
	default:
		ucplog.FromContextOrDiscard(r.Context()).Error(err, "Policy management failed; a write may require policy/history inspection before retrying")
		writeAuthorizationUnavailable(w, r)
	}
}
