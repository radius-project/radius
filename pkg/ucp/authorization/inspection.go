// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
)

// Access-check limits mirror authorization.tsp and bound work before policy access.
const (
	maxAccessChecks               = 100
	maxPrincipalIssuerLength      = 256
	maxPrincipalSubjectLength     = 512
	maxPrincipalDisplayNameLength = 128
	maxPermissionLength           = 256
	maxAccessScopeLength          = 2048
)

var (
	// ErrCheckAccessDenied means the caller cannot inspect another principal.
	ErrCheckAccessDenied = errors.New("checking another principal requires Radius.Core/checkAccess/action")
	// ErrMembershipUnavailable prevents reporting incomplete evaluations as denials.
	ErrMembershipUnavailable = errors.New("trusted group membership for another user or workload is unavailable; check as that caller or inspect a group")
)

// inspectPolicy captures one immutable snapshot without blocking refresh during
// evaluation. Its age and lifetime are rechecked before publishing any result.
func (r *PolicyRuntime) inspectPolicy(inspect func(*PolicySnapshot) error) error {
	if r == nil {
		return ErrAuthorizationUnavailable
	}
	r.mu.RLock()
	lifetime := r.ctx
	r.mu.RUnlock()
	r.cache.mu.RLock()
	snapshot, asOf := r.cache.value, r.cache.asOf
	r.cache.mu.RUnlock()
	fresh := func() bool {
		return lifetime != nil && snapshot != nil &&
			r.cache.clock.Since(asOf) < PolicyMaxAge && lifetime.Err() == nil
	}
	if !fresh() {
		return ErrAuthorizationUnavailable
	}
	err := inspect(snapshot)
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
	checks, err := validateAccessCheck(scope, request)
	if err != nil {
		return api.CheckAccessResponse{}, err
	}
	var result api.CheckAccessResponse
	err = r.inspectPolicy(func(snapshot *PolicySnapshot) error {
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

func validateAccessCheck(scope string, request api.AccessCheckRequest) ([]Check, error) {
	if len(request.Checks) == 0 || len(request.Checks) > maxAccessChecks {
		return nil, fmt.Errorf("checks must contain between 1 and %d items", maxAccessChecks)
	}
	if err := validateAccessString("scope", scope, maxAccessScopeLength); err != nil {
		return nil, err
	}
	if err := ValidateScope(scope); err != nil {
		return nil, err
	}
	if p := request.Principal; p != nil {
		for _, field := range []struct {
			name  string
			value *string
			limit int
		}{
			{"principal.issuer", p.Issuer, maxPrincipalIssuerLength},
			{"principal.subject", p.Subject, maxPrincipalSubjectLength},
			{"principal.displayName", p.DisplayName, maxPrincipalDisplayNameLength},
		} {
			if field.value != nil {
				if err := validateAccessString(field.name, *field.value, field.limit); err != nil {
					return nil, err
				}
			}
		}
		// The closed enum also rejects arbitrarily long type strings.
		if _, err := key(p); err != nil {
			return nil, err
		}
	}
	checks := make([]Check, 0, len(request.Checks))
	for _, check := range request.Checks {
		if check == nil || check.Permission == nil || check.Target == nil {
			return nil, fmt.Errorf("each check requires permission and target")
		}
		if err := validateAccessString("permission", *check.Permission, maxPermissionLength); err != nil {
			return nil, err
		}
		if err := validateAccessString("target", *check.Target, maxAccessScopeLength); err != nil {
			return nil, err
		}
		contained, err := ScopeContains(scope, *check.Target)
		if err != nil {
			return nil, err
		}
		if !contained {
			return nil, fmt.Errorf("check target must be within the inspection scope")
		}
		checks = append(checks, Check{*check.Permission, *check.Target})
	}
	return checks, nil
}

func validateAccessString(field, value string, limit int) error {
	if utf8.RuneCountInString(value) > limit {
		return fmt.Errorf("%s must contain at most %d characters", field, limit)
	}
	return nil
}
