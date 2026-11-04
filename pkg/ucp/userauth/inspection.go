// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package userauth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/authorization"
)

func validateAccessCheck(input api.AccessCheckRequest) error {
	if len(input.Checks) == 0 || len(input.Checks) > 100 {
		return fmt.Errorf("checks must contain between 1 and 100 items")
	}
	validate := func(name string, value *string, limit int) error {
		if value != nil && utf8.RuneCountInString(*value) > limit {
			return fmt.Errorf("%s must contain at most %d characters", name, limit)
		}
		return nil
	}
	if p := input.Principal; p != nil {
		for _, field := range []struct {
			name  string
			value *string
			limit int
		}{
			{"principal.issuer", p.Issuer, 256},
			{"principal.subject", p.Subject, 512},
			{"principal.displayName", p.DisplayName, 128},
		} {
			if err := validate(field.name, field.value, field.limit); err != nil {
				return err
			}
		}
	}
	for _, check := range input.Checks {
		if check == nil || check.Permission == nil || check.Target == nil {
			return fmt.Errorf("checks require permission and target")
		}
		if err := validate("permission", check.Permission, 256); err != nil {
			return err
		}
		if err := validate("target", check.Target, 2048); err != nil {
			return err
		}
	}
	return nil
}

func assignmentScope(value api.RoleAssignmentResource) string {
	id := strings.ToLower(*value.ID)
	scope := id[:strings.LastIndex(id, "/providers/radius.core/roleassignments/")]
	if scope == "" {
		return "/"
	}
	return scope
}

func canReadAssignments(snapshot *authorization.PolicySnapshot, identity authorization.Identity, scope string) (bool, error) {
	decisions, err := snapshot.Evaluate(identity, []authorization.Check{{
		Permission: "Radius.Core/roleAssignments/read", Target: scope,
	}})
	if err != nil {
		return false, err
	}
	return decisions[0].Allowed, nil
}

func (r *Runtime) whoami(policy authorization.Policy, identity authorization.Identity) api.WhoAmIResult {
	admin := false
	for _, value := range policy.Assignments {
		if !strings.EqualFold(*value.Properties.RoleDefinitionID, "/providers/Radius.Core/roleDefinitions/radius-administrator") {
			continue
		}
		admin = admin || samePrincipal(value.Properties.Principal, &identity.Principal)
		for i := range identity.Groups {
			admin = admin || samePrincipal(value.Properties.Principal, &identity.Groups[i])
		}
	}
	groups := make([]*api.Principal, len(identity.Groups))
	for i := range identity.Groups {
		groups[i] = &identity.Groups[i]
	}
	return api.WhoAmIResult{
		Principal: &identity.Principal, Groups: groups, Issuer: identity.Principal.Issuer,
		Mode: policy.Settings.Properties.Mode, IsRadiusAdministrator: to.Ptr(admin),
	}
}

func (r *Runtime) effectiveAccess(w http.ResponseWriter, req *http.Request, identity authorization.Identity, snapshot *authorization.PolicySnapshot, policy authorization.Policy, mapping authorization.RequestMapping) {
	if err := r.check(req, snapshot, identity, mapping.Checks, api.UserAuthorizationModeEnforce); err != nil {
		fail(w, err)
		return
	}
	scope := mapping.Checks[0].Target
	subject := identity
	if raw := req.URL.Query().Get("principal"); raw != "" {
		var principal api.Principal
		if err := json.Unmarshal([]byte(raw), &principal); err != nil {
			fail(w, fmt.Errorf("Invalid: principal must be a JSON principal: %w", err))
			return
		}
		if !samePrincipal(&principal, &identity.Principal) {
			subject = authorization.Identity{Principal: principal}
		}
	}
	// Evaluation validates the principal and its trusted groups even if no grants match.
	if _, err := snapshot.Evaluate(subject, []authorization.Check{{Permission: "Radius.Core/roleAssignments/read", Target: scope}}); err != nil {
		fail(w, err)
		return
	}
	roles := map[string]api.RoleDefinitionResource{}
	for _, role := range append(r.Catalog.BuiltInRoles(), policy.CustomRoles...) {
		roles[strings.ToLower(*role.ID)] = role
	}
	assignments := []*api.EffectiveAssignment{}
	permissions := map[string]string{}
	for i := range policy.Assignments {
		value := &policy.Assignments[i]
		parent := assignmentScope(*value)
		applies, err := authorization.ScopeContains(parent, scope)
		if err != nil {
			fail(w, err)
			return
		}
		matches := samePrincipal(value.Properties.Principal, &subject.Principal)
		for j := range subject.Groups {
			matches = matches || samePrincipal(value.Properties.Principal, &subject.Groups[j])
		}
		if !applies || !matches {
			continue
		}
		readable, err := canReadAssignments(snapshot, identity, parent)
		if err != nil {
			fail(w, err)
			return
		}
		if !readable {
			fail(w, fmt.Errorf("AuthorizationFailed: Radius.Core/roleAssignments/read at %s is required to inspect inherited grants", parent))
			return
		}
		assignments = append(assignments, &api.EffectiveAssignment{Assignment: value, Inherited: to.Ptr(!strings.EqualFold(parent, scope))})
		role := roles[strings.ToLower(*value.Properties.RoleDefinitionID)]
		for _, permission := range role.Properties.Permissions {
			permissions[strings.ToLower(*permission)] = *permission
		}
	}
	names := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		names = append(names, permission)
	}
	slices.Sort(names)
	values := make([]*string, len(names))
	for i := range names {
		values[i] = &names[i]
	}
	write(w, http.StatusOK, api.EffectiveAccessResponse{
		Principal: &subject.Principal, Scope: &scope, Generation: to.Ptr(policy.Generation),
		Assignments: assignments, Permissions: values,
	})
}

func explainedChecks(snapshot *authorization.PolicySnapshot, identity authorization.Identity, policy authorization.Policy, decisions []authorization.Decision) ([]*api.ExplainedAccessCheck, error) {
	results := make([]*api.ExplainedAccessCheck, 0, len(decisions))
	for _, decision := range decisions {
		result := &api.ExplainedAccessCheck{
			Allowed: to.Ptr(decision.Allowed), Permission: to.Ptr(decision.Permission), Target: to.Ptr(decision.Target),
		}
		for _, assignment := range policy.Assignments {
			if !strings.EqualFold(*assignment.ID, decision.AssignmentID) {
				continue
			}
			scope := assignmentScope(assignment)
			readable, err := canReadAssignments(snapshot, identity, scope)
			if err != nil {
				return nil, err
			}
			if readable {
				result.Grants = []*api.AccessGrant{{
					AssignmentID: assignment.ID, RoleDefinitionID: assignment.Properties.RoleDefinitionID, Scope: &scope,
				}}
			}
		}
		results = append(results, result)
	}
	return results, nil
}
