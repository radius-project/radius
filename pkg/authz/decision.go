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

	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	// LogFieldMode is the log key for the authorization mode.
	LogFieldMode = "authzMode"

	// LogFieldWouldDeny marks a denial that was logged in dry-run mode instead of rejected.
	LogFieldWouldDeny = "authzWouldDeny"

	// LogFieldCode is the log key for the authorization error code.
	LogFieldCode = "authzCode"

	// LogFieldReason is the log key for the authorization denial reason.
	LogFieldReason = "authzReason"
)

// Decision is the result of an authorization check.
type Decision struct {
	// Allowed is true when the check passed.
	Allowed bool

	// Code is the error code for a denial, such as "AuthorizationFailed".
	Code string

	// Reason describes why the check denied the request.
	Reason string
}

// DeniedError is returned by Apply when a denial is enforced.
type DeniedError struct {
	// Code is the error code for the denial.
	Code string

	// Reason describes why the request was denied.
	Reason string
}

// Error implements the error interface.
func (e *DeniedError) Error() string {
	return fmt.Sprintf("authorization denied (%s): %s", e.Code, e.Reason)
}

// CodeAuthorizationFailed is used when a denial does not carry a specific code.
const CodeAuthorizationFailed = "AuthorizationFailed"

// Apply acts on an authorization decision according to mode. It returns nil when the
// decision is allowed or mode is off, logs and returns nil for a denial in dry-run mode,
// and returns a *DeniedError for a denial in enforce mode. An unknown mode fails closed,
// even for allowed decisions.
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
			LogFieldReason, decision.Reason)
		return nil
	}

	return &DeniedError{Code: code, Reason: decision.Reason}
}
