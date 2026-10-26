// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package userauth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz/records"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/authorization"
	"github.com/radius-project/radius/pkg/ucp/authzcheck"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const maxBody = 4 * 1024 * 1024

// Middleware requires a verified caller before either policy or workload routing.
func (r *Runtime) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		path := strings.TrimPrefix(req.URL.Path, r.PathBase)
		lower := strings.ToLower(path)
		// Discovery contains no user data. Never use the internal prototype's broad
		// discovery predicate, which also classifies installation policy routes.
		if req.Method == http.MethodGet && slices.Contains([]string{"", "/", "/version", "/openapi/v2", "/openapi/v3"}, lower) {
			next.ServeHTTP(w, req)
			return
		}
		caller, _ := authzcheck.CallerFromContext(req.Context())
		if caller.Kind == authzcheck.CallerComponent {
			r.component(w, req, next, path)
			return
		}
		identity, err := r.identity(req.Context())
		if err != nil {
			fail(w, err)
			return
		}
		// Client-supplied delegation IDs cannot become server-verified provenance.
		req.Header.Del(records.HeaderRecord)
		req.Header.Del(records.HeaderOperation)
		req.Header.Del(records.HeaderSourceNamespace)
		if lower == initializePath && req.Method == http.MethodPost {
			r.initialize(w, req, identity)
			return
		}
		snapshot, err := r.Cache.Snapshot()
		if err != nil {
			fail(w, err)
			return
		}
		policy, err := snapshot.Policy()
		if err != nil {
			fail(w, err)
			return
		}
		mode := *policy.Settings.Properties.Mode
		if lower == "/providers/radius.core/whoami" && req.Method == http.MethodGet {
			write(w, http.StatusOK, map[string]any{"principal": identity.Principal, "groups": identity.Groups, "mode": mode, "generation": policy.Generation})
			return
		}
		mapping, err := r.Catalog.MapRequest(req.Method, path)
		if err != nil {
			fail(w, fmt.Errorf("AuthorizationIndeterminate: %w", err))
			return
		}
		if r.policyAPI(w, req, path, identity, snapshot, policy, mapping) {
			return
		}
		if mode == api.UserAuthorizationModeDisabled {
			next.ServeHTTP(w, req)
			return
		}
		if req.Method == http.MethodPost && strings.HasSuffix(lower, "/getgraph") && mode == api.UserAuthorizationModeEnforce {
			fail(w, fmt.Errorf("AuthorizationIndeterminate: cross-scope graph redaction is not supported by this POC"))
			return
		}
		if mapping.Kind == authorization.OperationStatusRequest {
			// Operation-target resolution is not yet implemented. Do not grant a
			// blanket status-read exception.
			err := fmt.Errorf("AuthorizationIndeterminate: operation status inspection is not supported by this POC; read the target resource")
			if mode == api.UserAuthorizationModeDryRun {
				ucplog.FromContextOrDiscard(req.Context()).Info("user authorization would deny", "wouldDeny", true, "reason", err.Error(), "target", path)
				next.ServeHTTP(w, req)
				return
			}
			fail(w, err)
			return
		}
		if mapping.Kind == authorization.MixedResourceListRequest {
			r.filteredList(w, req, next, identity, snapshot, mode)
			return
		}
		if mapping.Kind == authorization.ListRequest {
			decisions, err := snapshot.Evaluate(identity, mapping.Checks)
			if err != nil {
				fail(w, err)
				return
			}
			if len(decisions) > 0 && !decisions[0].Allowed && mode == api.UserAuthorizationModeEnforce {
				r.filteredList(w, req, next, identity, snapshot, mode)
				return
			}
		}
		if err := r.check(req, snapshot, identity, mapping.Checks, mode); err != nil {
			fail(w, err)
			return
		}
		if req.Method == http.MethodPut || req.Method == http.MethodPatch {
			body, err := requestBody(req)
			if err != nil {
				fail(w, err)
				return
			}
			if req.Method == http.MethodPatch {
				body, err = effectivePatch(req, body, next)
				if err != nil {
					fail(w, err)
					return
				}
			}
			if strings.Contains(lower, "/providers/microsoft.resources/deployments/") {
				prepared, err := deploymentBody(path, body)
				if err != nil {
					if mode == api.UserAuthorizationModeEnforce {
						fail(w, err)
						return
					}
					ucplog.FromContextOrDiscard(req.Context()).Info("user authorization would deny", "wouldDeny", true, "reason", err.Error(), "target", path)
					next.ServeHTTP(w, req)
					return
				}
				body = prepared
				setBody(req, body)
			}
			checks, approvals, err := r.writeChecks(path, body)
			if err != nil {
				if mode == api.UserAuthorizationModeEnforce {
					fail(w, err)
					return
				}
				ucplog.FromContextOrDiscard(req.Context()).Info("user authorization would deny", "wouldDeny", true, "reason", err.Error(), "target", path)
			}
			if err == nil && strings.Contains(lower, "/providers/microsoft.resources/deployments/") {
				converted, conversionError := engineDeploymentBody(body)
				if conversionError != nil {
					if mode == api.UserAuthorizationModeEnforce {
						fail(w, conversionError)
						return
					}
					ucplog.FromContextOrDiscard(req.Context()).Info("user authorization would deny", "wouldDeny", true, "reason", conversionError.Error(), "target", path)
				} else {
					body = converted
					setBody(req, body)
					approvals[0].InputHash = records.InputHash(body)
				}
			}
			if err := r.check(req, snapshot, identity, checks, mode); err != nil {
				fail(w, err)
				return
			}
			rawIdentity, err := json.Marshal(identity)
			if err != nil {
				fail(w, err)
				return
			}
			req = req.WithContext(records.WithUserApproval(req.Context(), records.UserApproval{Identity: string(rawIdentity), Resources: approvals}))
		}
		next.ServeHTTP(w, req)
	})
}

func (r *Runtime) check(req *http.Request, snapshot *authorization.PolicySnapshot, identity authorization.Identity, checks []authorization.Check, mode api.UserAuthorizationMode) error {
	if err := r.Cache.Ready(); err != nil {
		return err
	}
	if len(checks) == 0 {
		return nil
	}
	decisions, err := snapshot.Evaluate(identity, checks)
	if err != nil {
		return err
	}
	for _, decision := range decisions {
		ucplog.FromContextOrDiscard(req.Context()).Info("user authorization decision",
			"principal", *identity.Principal.Subject, "groups", identity.Groups, "permission", decision.Permission,
			"target", decision.Target, "allowed", decision.Allowed, "wouldDeny", !decision.Allowed && mode == api.UserAuthorizationModeDryRun,
			"generation", snapshot.Generation(), "mode", mode, "requestId", req.Header.Get("X-Ms-Client-Request-Id"))
		if !decision.Allowed && mode == api.UserAuthorizationModeEnforce {
			return fmt.Errorf("AuthorizationFailed: %s at %s", decision.Permission, decision.Target)
		}
	}
	return nil
}

func requestBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, fmt.Errorf("request body required")
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, maxBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxBody {
		return nil, fmt.Errorf("request exceeds %d bytes", maxBody)
	}
	if err := req.Body.Close(); err != nil {
		return nil, err
	}
	setBody(req, body)
	return body, nil
}

func setBody(req *http.Request, body []byte) {
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
}

func effectivePatch(req *http.Request, patch []byte, next http.Handler) ([]byte, error) {
	get := req.Clone(req.Context())
	get.Method, get.Body, get.ContentLength = http.MethodGet, nil, 0
	get.Header = req.Header.Clone()
	get.Header.Del("Content-Type")
	response := httptest.NewRecorder()
	next.ServeHTTP(response, get)
	if response.Code != http.StatusOK {
		return nil, fmt.Errorf("AuthorizationIndeterminate: load effective PATCH resource returned HTTP %d", response.Code)
	}
	body, err := jsonpatch.MergePatch(response.Body.Bytes(), patch)
	if err != nil {
		return nil, fmt.Errorf("Invalid: merge PATCH: %w", err)
	}
	if len(body) > maxBody {
		return nil, fmt.Errorf("Invalid: effective PATCH exceeds %d bytes", maxBody)
	}
	if etag := response.Header().Get("ETag"); etag != "" {
		req.Header.Set("If-Match", etag)
	}
	// The RP's current PATCH handlers use replacement semantics. Forward the
	// complete effective document so enforcement and persistence see the same input.
	setBody(req, body)
	return body, nil
}

func (r *Runtime) policyAPI(w http.ResponseWriter, req *http.Request, path string, identity authorization.Identity, snapshot *authorization.PolicySnapshot, policy authorization.Policy, mapping authorization.RequestMapping) bool {
	lower := strings.ToLower(path)
	isRole := strings.Contains(lower, "/providers/radius.core/roledefinitions")
	isAssignment := strings.Contains(lower, "/providers/radius.core/roleassignments")
	isSettings := lower == settingsPath
	isChange := strings.HasPrefix(lower, "/providers/radius.core/accesschanges")
	isPermission := lower == "/providers/radius.core/permissions"
	isAccess := strings.HasSuffix(lower, "/checkaccess") || strings.HasSuffix(lower, "/explainaccess")
	if !isRole && !isAssignment && !isSettings && !isChange && !isPermission && !isAccess {
		return false
	}
	if isAccess && req.Method == http.MethodPost {
		var input api.AccessCheckRequest
		body, err := requestBody(req)
		if err == nil {
			err = json.Unmarshal(body, &input)
		}
		if err != nil {
			fail(w, err)
			return true
		}
		subject := identity
		if input.Principal != nil && !samePrincipal(input.Principal, &identity.Principal) {
			if err := r.check(req, snapshot, identity, mapping.Checks, api.UserAuthorizationModeEnforce); err != nil {
				fail(w, err)
				return true
			}

			subject = authorization.Identity{Principal: *input.Principal}
		}
		checks := []authorization.Check{}
		for _, check := range input.Checks {
			if check == nil || check.Permission == nil || check.Target == nil {
				fail(w, fmt.Errorf("checks require permission and target"))
				return true
			}
			checks = append(checks, authorization.Check{Permission: *check.Permission, Target: *check.Target})
		}
		decisions, err := snapshot.Evaluate(subject, checks)
		if err != nil {
			fail(w, err)
			return true
		}
		results := make([]api.AccessCheckResult, 0, len(decisions))
		for _, decision := range decisions {
			results = append(results, api.AccessCheckResult{Allowed: to.Ptr(decision.Allowed), Permission: to.Ptr(decision.Permission), Target: to.Ptr(decision.Target)})
		}
		// This POC deliberately redacts grant IDs for all callers.
		write(w, http.StatusOK, map[string]any{"principal": subject.Principal, "mode": policy.Settings.Properties.Mode, "generation": policy.Generation, "checks": results})
		return true
	}
	if req.Method != http.MethodGet {
		mutation := authorization.PolicyMutation{Target: path, Delete: req.Method == http.MethodDelete}
		if !mutation.Delete {
			if req.Method != http.MethodPut {
				write(w, http.StatusMethodNotAllowed, map[string]string{"error": "use PUT or DELETE for policy resources"})
				return true
			}
			body, err := requestBody(req)
			if err != nil {
				fail(w, err)
				return true
			}
			switch {
			case isRole:
				var resource api.RoleDefinitionResource
				err = json.Unmarshal(body, &resource)
				mutation.Role = resource.Properties
			case isAssignment:
				var resource api.RoleAssignmentResource
				err = json.Unmarshal(body, &resource)
				mutation.Assignment = resource.Properties
				if resource.Properties != nil && (resource.Properties.ManagedBy != nil || resource.Properties.CreatedBy != nil || resource.Properties.Scope != nil) {
					err = fmt.Errorf("ManagedByConflict: provenance and scope are server-owned")
				}
			case isSettings:
				var resource api.AuthorizationSettingsResource
				err = json.Unmarshal(body, &resource)
				mutation.Settings = resource.Properties
			default:
				err = fmt.Errorf("resource is read-only")
			}
			if err != nil {
				fail(w, err)
				return true
			}
		}
		actor := authorization.PolicyActor{Identity: identity, RequestID: uuid.NewString(),
			CorrelationID: req.Header.Get("X-Ms-Correlation-Request-Id"), Management: api.AssignmentManagement{Kind: to.Ptr(api.AssignmentManagementKindImperative)}}
		result, err := r.Store.MutateAndRefresh(req.Context(), r.Cache, actor, mutation)
		if err != nil {
			fail(w, err)
			return true
		}
		ucplog.FromContextOrDiscard(req.Context()).Info("user policy changed", "generation", result.Generation(), "requestId", actor.RequestID, "principal", *identity.Principal.Subject, "target", path)
		if mutation.Delete {
			w.WriteHeader(http.StatusNoContent)
			return true
		}
		updated, err := result.Policy()
		if err != nil {
			fail(w, err)
			return true
		}
		r.policyRead(w, req, lower, updated, isRole, isAssignment, isSettings, false, false)
		return true
	}
	if err := r.check(req, snapshot, identity, mapping.Checks, api.UserAuthorizationModeEnforce); err != nil {
		fail(w, err)
		return true
	}
	r.policyRead(w, req, lower, policy, isRole, isAssignment, isSettings, isChange, isPermission)
	return true
}

func samePrincipal(a, b *api.Principal) bool {
	return a != nil && b != nil && a.Type != nil && b.Type != nil &&
		a.Issuer != nil && b.Issuer != nil && a.Subject != nil && b.Subject != nil &&
		*a.Type == *b.Type && *a.Issuer == *b.Issuer && *a.Subject == *b.Subject
}

func (r *Runtime) policyRead(w http.ResponseWriter, req *http.Request, path string, policy authorization.Policy, role, assignment, settings, change, permissions bool) {
	switch {
	case settings:
		write(w, http.StatusOK, policy.Settings)
	case permissions:
		values := []map[string]string{}
		for _, permission := range r.Catalog.Permissions() {
			values = append(values, map[string]string{"name": permission})
		}
		write(w, http.StatusOK, map[string]any{"value": values})
	case change:
		if strings.HasSuffix(path, "/accesschanges") {
			// Bound the response; fetch older generations individually.
			values := []*api.AccessChangeResource{}
			for generation := max(int64(1), policy.Generation-99); generation <= policy.Generation; generation++ {
				event, err := r.Store.AccessChange(req.Context(), generation)
				if err != nil {
					fail(w, err)
					return
				}
				values = append(values, event)
			}
			write(w, http.StatusOK, map[string]any{"value": values})
			return
		}
		generation, err := strconv.ParseInt(path[strings.LastIndex(path, "/")+1:], 10, 64)
		if err != nil {
			fail(w, err)
			return
		}
		event, err := r.Store.AccessChange(req.Context(), generation)
		if err != nil {
			fail(w, err)
			return
		}
		write(w, http.StatusOK, event)
	case role:
		values := []api.RoleDefinitionResource{}
		for _, value := range append(r.Catalog.BuiltInRoles(), policy.CustomRoles...) {
			if strings.EqualFold(*value.ID, path) {
				write(w, http.StatusOK, value)
				return
			}
			if strings.EqualFold((*value.ID)[:strings.LastIndex(*value.ID, "/")], path) {
				values = append(values, value)
			}
		}
		if strings.HasSuffix(path, "/roledefinitions") {
			write(w, http.StatusOK, map[string]any{"value": values})
		} else {
			write(w, http.StatusNotFound, map[string]any{"error": map[string]string{"code": "NotFound"}})
		}
	case assignment:
		values := []api.RoleAssignmentResource{}
		for _, value := range policy.Assignments {
			if strings.EqualFold(*value.ID, path) {
				write(w, http.StatusOK, value)
				return
			}
			if strings.EqualFold((*value.ID)[:strings.LastIndex(*value.ID, "/")], path) {
				values = append(values, value)
			}
		}
		if strings.HasSuffix(path, "/roleassignments") {
			write(w, http.StatusOK, map[string]any{"value": values})
		} else {
			write(w, http.StatusNotFound, map[string]any{"error": map[string]string{"code": "NotFound"}})
		}
	}
}

func (r *Runtime) filteredList(w http.ResponseWriter, req *http.Request, next http.Handler, identity authorization.Identity, snapshot *authorization.PolicySnapshot, mode api.UserAuthorizationMode) {
	response := httptest.NewRecorder()
	next.ServeHTTP(response, req)
	if response.Code < 200 || response.Code >= 300 {
		for key, values := range response.Header() {
			w.Header()[key] = values
		}
		w.WriteHeader(response.Code)
		_, err := w.Write(response.Body.Bytes())
		if err != nil {
			ucplog.FromContextOrDiscard(req.Context()).Error(err, "write filtered list response")
		}
		return
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		fail(w, fmt.Errorf("AuthorizationIndeterminate: cannot filter list: %w", err))
		return
	}
	var values []json.RawMessage
	if err := json.Unmarshal(body["value"], &values); err != nil {
		fail(w, fmt.Errorf("AuthorizationIndeterminate: list has no value array"))
		return
	}
	filtered := []json.RawMessage{}
	for _, value := range values {
		var item struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(value, &item); err != nil || item.ID == "" {
			fail(w, fmt.Errorf("AuthorizationIndeterminate: list item has no resource ID"))
			return
		}
		mapping, err := r.Catalog.MapRequest(http.MethodGet, item.ID)
		if err != nil {
			fail(w, fmt.Errorf("AuthorizationIndeterminate: %w", err))
			return
		}
		decisions, err := snapshot.Evaluate(identity, mapping.Checks)
		if err != nil {
			fail(w, err)
			return
		}
		if mode != api.UserAuthorizationModeEnforce || len(decisions) == 1 && decisions[0].Allowed {
			filtered = append(filtered, value)
		}
	}
	var err error
	body["value"], err = json.Marshal(filtered)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, response.Code, body)
}
