/*
Copyright 2026 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package authz

import (
	"context"
	"fmt"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	// LogFieldMode is the log key for the authorization mode.
	LogFieldMode = "authzMode"

	// LogFieldWouldDeny marks a denial that was logged in dry-run mode instead of rejected.
	LogFieldWouldDeny = "authzWouldDeny"

	// LogFieldDenied marks a denial that was rejected in enforce mode.
	LogFieldDenied = "authzDenied"

	// LogFieldCode is the log key for the authorization error code.
	LogFieldCode = "authzCode"

	// LogFieldAction is the log key for the caller's requested action.
	LogFieldAction = "authzAction"

	// LogFieldTarget is the log key for the caller's requested target.
	LogFieldTarget = "authzTarget"

	// LogFieldRule is the log key for the public admission rule identifier.
	LogFieldRule = "authzRule"
)

// Decision is the result of an authorization check.
type Decision struct {
	// Allowed is true when the check passed.
	Allowed bool

	// Code is the error code for a denial, such as "AuthorizationFailed".
	Code string

	// Action is the caller's requested action, such as "Applications.Core/containers/write".
	Action string

	// Target is the caller's requested resource ID or scope, not a resource discovered
	// while evaluating permissions.
	Target string

	// Rule is the public identifier of the admission rule that rejected the request,
	// such as "control-plane-secret". It is required for AdmissionPolicyDenied.
	Rule string
}

// DeniedError is returned by Apply when a denial is enforced.
type DeniedError struct {
	// Code is the error code for the denial.
	Code string

	// Action is the caller's requested action. It is required for ARM responses.
	Action string

	// Target is the caller's requested resource ID or scope, not a resource discovered
	// while evaluating permissions. It is required for ARM responses.
	Target string

	// Rule is the public admission rule identifier. It is required for, and only
	// included in, AdmissionPolicyDenied responses.
	Rule string
}

// Error implements the error interface.
func (e *DeniedError) Error() string {
	code := e.Code
	if code == "" {
		code = CodeAuthorizationFailed
	}

	message := fmt.Sprintf("authorization denied (%s)", code)
	if e.Action != "" {
		message += fmt.Sprintf(" for action %q", e.Action)
	}
	if e.Target != "" {
		message += fmt.Sprintf(" on target %q", e.Target)
	}

	return message
}

// CodeAuthorizationFailed is used when a denial does not carry a specific code.
const CodeAuthorizationFailed = v1.CodeAuthorizationFailed

// Apply acts on an authorization decision according to mode. It returns nil when the
// decision is allowed or mode is off, logs and returns nil for a denial in dry-run mode,
// and logs and returns a *DeniedError for a denial in enforce mode. An unknown mode
// fails closed, even for allowed decisions.
func Apply(ctx context.Context, mode Mode, decision Decision) error {
	switch mode {
	case "", ModeOff, ModeDryRun, ModeEnforce:
	default:
		return fmt.Errorf("invalid authorization mode %q", mode)
	}

	if decision.Allowed || mode == "" || mode == ModeOff {
		return nil
	}

	code := decision.Code
	if code == "" {
		code = CodeAuthorizationFailed
	}

	if mode == ModeDryRun {
		ucplog.FromContextOrDiscard(ctx).Info("authorization check would deny request",
			LogFieldWouldDeny, true,
			LogFieldCode, code,
			LogFieldAction, decision.Action,
			LogFieldTarget, decision.Target,
			LogFieldRule, decision.Rule)
		return nil
	}

	ucplog.FromContextOrDiscard(ctx).Info("authorization check denied request",
		LogFieldDenied, true,
		LogFieldCode, code,
		LogFieldAction, decision.Action,
		LogFieldTarget, decision.Target,
		LogFieldRule, decision.Rule)

	return &DeniedError{Code: code, Action: decision.Action, Target: decision.Target, Rule: decision.Rule}
}
