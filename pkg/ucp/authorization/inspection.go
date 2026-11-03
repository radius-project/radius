// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
)

var (
	// ErrCheckAccessDenied means the caller cannot inspect another principal.
	ErrCheckAccessDenied = errors.New("checking another principal requires Radius.Core/checkAccess/action")
	// ErrMembershipUnavailable prevents reporting incomplete evaluations as denials.
	ErrMembershipUnavailable = errors.New("trusted group membership for another user or workload is unavailable; check as that caller or inspect a group")
)

// inspectPolicy holds the runtime and cache locks for the complete operation.
// Neither the snapshot nor a permission decision escapes across a freshness gate.
func (r *PolicyRuntime) inspectPolicy(inspect func(*PolicySnapshot) error) error {
	if r == nil {
		return ErrAuthorizationUnavailable
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	r.cache.mu.RLock()
	defer r.cache.mu.RUnlock()
	fresh := func() bool {
		return r.ctx != nil && r.ctx.Err() == nil && r.cache.value != nil &&
			r.cache.clock.Since(r.cache.asOf) < PolicyMaxAge
	}
	if !fresh() {
		return ErrAuthorizationUnavailable
	}
	err := inspect(r.cache.value)
	if !fresh() {
		return ErrAuthorizationUnavailable
	}
	return err
}

// WhoAmI returns identity and installation-administrator status from fresh policy.
// The caller must supply only identity from the verified authentication boundary.
func (r *PolicyRuntime) WhoAmI(identity Identity) (api.WhoAmIResult, error) {
	var result api.WhoAmIResult
	err := r.inspectPolicy(func(snapshot *PolicySnapshot) error {
		if _, err := snapshot.Evaluate(identity, []Check{{"Radius.Core/checkAccess/action", "/"}}); err != nil {
			return err
		}
		principals := map[principalKey]bool{}
		for _, principal := range append([]api.Principal{identity.Principal}, identity.Groups...) {
			k, err := key(&principal)
			if err != nil {
				return err
			}
			principals[k] = true
		}
		admin := false
		for _, assignment := range snapshot.evaluator.assignments {
			if principals[assignment.principal] && assignment.scope == "/" &&
				assignment.role.builtin && assignment.role.name == "radius-administrator" {
				admin = true
			}
		}
		result = api.WhoAmIResult{
			Principal: &identity.Principal, Issuer: identity.Principal.Issuer,
			Groups: make([]*api.Principal, 0, len(identity.Groups)),
			Mode:   to.Ptr(*snapshot.policy.Settings.Properties.Mode), IsRadiusAdministrator: to.Ptr(admin),
			Warnings: []*string{},
		}
		for _, group := range identity.Groups {
			result.Groups = append(result.Groups, to.Ptr(group))
		}
		switch *result.Mode {
		case api.UserAuthorizationModeDisabled:
			result.Warnings = append(result.Warnings, to.Ptr("User authorization is Disabled; workload requests are not checked."))
		case api.UserAuthorizationModeDryRun:
			result.Warnings = append(result.Warnings, to.Ptr("DryRun observes request targets only; denied checks do not block workload requests."))
		case api.UserAuthorizationModeEnforce:
			result.Warnings = append(result.Warnings, to.Ptr("Workload enforcement rollout is unsupported; workload requests are unavailable."))
		}
		if permanentAdministrators(snapshot.policy) == 0 {
			result.Warnings = append(result.Warnings, to.Ptr("No permanent user or group installation administrator is configured; bootstrap grants alone are not sufficient for enforcement."))
		}
		result.Warnings = append(result.Warnings, to.Ptr("Recovery grant management is not available in this release."))
		return nil
	})
	if err != nil {
		return api.WhoAmIResult{}, err
	}
	return result, nil
}

// CheckAccess evaluates a batch and its inspection permission at one generation,
// independently of workload mode. Group inspection uses only that group's grants.
// Another user's memberships cannot be asserted or inferred from request input.
func (r *PolicyRuntime) CheckAccess(caller Identity, scope string, request api.AccessCheckRequest) (api.CheckAccessResponse, error) {
	var result api.CheckAccessResponse
	err := r.inspectPolicy(func(snapshot *PolicySnapshot) error {
		if err := ValidateScope(scope); err != nil {
			return err
		}
		if len(request.Checks) == 0 {
			return fmt.Errorf("at least one access check is required")
		}
		checks := make([]Check, 0, len(request.Checks))
		for _, check := range request.Checks {
			if check == nil || check.Permission == nil || check.Target == nil {
				return fmt.Errorf("each check requires permission and target")
			}
			contained, err := ScopeContains(scope, *check.Target)
			if err != nil {
				return err
			}
			if !contained {
				return fmt.Errorf("check target must be within the inspection scope")
			}
			checks = append(checks, Check{*check.Permission, *check.Target})
		}
		identity := caller
		if request.Principal != nil {
			subject, err := key(request.Principal)
			if err != nil {
				return err
			}
			self, err := key(&caller.Principal)
			if err != nil {
				return err
			}
			if subject != self {
				gate, err := snapshot.Evaluate(caller, []Check{{"Radius.Core/checkAccess/action", scope}})
				if err != nil {
					return err
				}
				if !gate[0].Allowed {
					return ErrCheckAccessDenied
				}
				if subject.typ != api.PrincipalTypeGroup {
					return ErrMembershipUnavailable
				}
				identity = Identity{Principal: *request.Principal}
			}
		}
		decisions, err := snapshot.Evaluate(identity, checks)
		if err != nil {
			return err
		}
		result = api.CheckAccessResponse{
			Principal: &identity.Principal, Generation: to.Ptr(snapshot.Generation()),
			Mode:   to.Ptr(*snapshot.policy.Settings.Properties.Mode),
			Checks: make([]*api.AccessCheckResult, 0, len(decisions)),
		}
		for _, decision := range decisions {
			result.Checks = append(result.Checks, &api.AccessCheckResult{
				Permission: to.Ptr(decision.Permission), Target: to.Ptr(decision.Target), Allowed: to.Ptr(decision.Allowed),
			})
		}
		return nil
	})
	if err != nil {
		return api.CheckAccessResponse{}, err
	}
	return result, nil
}
