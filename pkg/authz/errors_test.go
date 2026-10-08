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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
)

func Test_StatusForCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code string
		want int
	}{
		{code: v1.CodeInvalidAuthenticationInfo, want: http.StatusUnauthorized},
		{code: v1.CodeAuthorizationFailed, want: http.StatusForbidden},
		{code: v1.CodeExecutionRecordNotActive, want: http.StatusForbidden},
		{code: v1.CodeGrantScopeExceeded, want: http.StatusForbidden},
		{code: v1.CodeOperationNotAssigned, want: http.StatusForbidden},
		{code: v1.CodeOperationInputMismatch, want: http.StatusConflict},
		{code: v1.CodeQueueWaitLimitExceeded, want: http.StatusForbidden},
		{code: v1.CodeCredentialIssuanceDenied, want: http.StatusForbidden},
		{code: v1.CodeAdmissionPolicyDenied, want: http.StatusForbidden},
		{code: v1.CodeAuthorizationUnavailable, want: http.StatusServiceUnavailable},
		{code: CodePeerCertificateInvalid, want: http.StatusInternalServerError},
		{code: CodeCertificateExpired, want: http.StatusInternalServerError},
		{code: "", want: http.StatusInternalServerError},
		{code: "SomethingElse", want: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, StatusForCode(tt.code))
		})
	}
}

func Test_Apply_DeniedErrorCarriesActionAndTarget(t *testing.T) {
	t.Parallel()

	decision := Decision{
		Code:   v1.CodeGrantScopeExceeded,
		Reason: "target is outside the record",
		Action: "Applications.Core/containers/write",
		Target: "/planes/radius/local/resourceGroups/rg/providers/Applications.Core/containers/c",
	}

	err := Apply(t.Context(), ModeEnforce, decision)

	var denied *DeniedError
	require.ErrorAs(t, err, &denied)
	require.Equal(t, decision.Action, denied.Action)
	require.Equal(t, decision.Target, denied.Target)
}

func Test_Apply_DeniedErrorCarriesRule(t *testing.T) {
	t.Parallel()

	decision := Decision{
		Code:   v1.CodeAdmissionPolicyDenied,
		Action: "Applications.Core/containers/write",
		Target: "/planes/radius/local/resourceGroups/rg/providers/Applications.Core/containers/c",
		Rule:   "control-plane-secret",
	}

	var denied *DeniedError
	require.ErrorAs(t, Apply(t.Context(), ModeEnforce, decision), &denied)
	require.Equal(t, decision.Rule, denied.Rule)
}

func Test_DeniedError_AdmissionPolicyDeniedNamesRule(t *testing.T) {
	t.Parallel()

	const (
		action = "Applications.Core/containers/write"
		target = "/planes/radius/local/resourceGroups/rg/providers/Applications.Core/containers/c"
	)
	denied := &DeniedError{
		Code:   v1.CodeAdmissionPolicyDenied,
		Reason: "pod mounts secret radius-system/ucp-cert owned by bob@example.com",
		Action: action,
		Target: target,
		Rule:   "control-plane-secret",
	}

	body, err := denied.ErrorResponse()
	require.NoError(t, err)
	require.Equal(t, v1.CodeAdmissionPolicyDenied, body.Error.Code)
	require.Equal(t, target, body.Error.Target)
	require.Equal(t, "Authorization denied for action '"+action+"' on target '"+target+"' by admission rule 'control-plane-secret'", body.Error.Message)

	response, err := denied.Response()
	require.NoError(t, err)
	w := httptest.NewRecorder()
	require.NoError(t, response.Apply(t.Context(), w, httptest.NewRequest(http.MethodPut, "/", nil)))
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "control-plane-secret")
	require.NotContains(t, w.Body.String(), "bob@example.com")
	require.NotContains(t, w.Body.String(), "ucp-cert")
}

func Test_DeniedError_AdmissionPolicyDeniedRequiresValidRule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		rule string
	}{
		{name: "missing"},
		{name: "blank", rule: " \t"},
		{name: "free text", rule: "denied because bob@example.com owns it"},
		{name: "email", rule: "bob@example.com"},
		{name: "quote", rule: "rule'injected"},
		{name: "too long", rule: strings.Repeat("a", 129)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			denied := &DeniedError{
				Code:   v1.CodeAdmissionPolicyDenied,
				Action: "read",
				Target: "/requested",
				Rule:   tt.rule,
			}
			body, err := denied.ErrorResponse()
			require.EqualError(t, err, "admission policy response requires a valid rule identifier")
			require.Equal(t, v1.ErrorResponse{}, body)
			response, err := denied.Response()
			require.EqualError(t, err, "admission policy response requires a valid rule identifier")
			require.Nil(t, response)
		})
	}
}

func Test_DeniedError_RuleOmittedForOtherCodes(t *testing.T) {
	t.Parallel()

	denied := &DeniedError{
		Code:   v1.CodeAuthorizationFailed,
		Action: "read",
		Target: "/requested",
		Rule:   "control-plane-secret",
	}
	body, err := denied.ErrorResponse()
	require.NoError(t, err)
	require.Equal(t, "Authorization denied for action 'read' on target '/requested'", body.Error.Message)
}

func Test_DeniedError_ErrorResponse(t *testing.T) {
	t.Parallel()

	const (
		action = "Applications.Core/containers/write"
		target = "/planes/radius/local/resourceGroups/rg/providers/Applications.Core/containers/c"
	)

	tests := []struct {
		name        string
		err         *DeniedError
		wantStatus  int
		wantCode    string
		wantMessage string
		wantTarget  string
	}{
		{
			name:        "forbidden with action and target",
			err:         &DeniedError{Code: v1.CodeGrantScopeExceeded, Reason: "target is outside the record", Action: action, Target: target},
			wantStatus:  http.StatusForbidden,
			wantCode:    v1.CodeGrantScopeExceeded,
			wantMessage: "Authorization denied for action '" + action + "' on target '" + target + "'",
			wantTarget:  target,
		},
		{
			name:        "conflict",
			err:         &DeniedError{Code: v1.CodeOperationInputMismatch, Reason: "input hash does not match", Action: action, Target: target},
			wantStatus:  http.StatusConflict,
			wantCode:    v1.CodeOperationInputMismatch,
			wantMessage: "Authorization denied for action '" + action + "' on target '" + target + "'",
			wantTarget:  target,
		},
		{
			name:        "unavailable",
			err:         &DeniedError{Code: v1.CodeAuthorizationUnavailable, Reason: "permission store unreachable", Action: action, Target: target},
			wantStatus:  http.StatusServiceUnavailable,
			wantCode:    v1.CodeAuthorizationUnavailable,
			wantMessage: "Authorization denied for action '" + action + "' on target '" + target + "'",
			wantTarget:  target,
		},
		{
			name:        "empty code defaults to AuthorizationFailed",
			err:         &DeniedError{Reason: "no permission", Action: action, Target: target},
			wantStatus:  http.StatusForbidden,
			wantCode:    v1.CodeAuthorizationFailed,
			wantMessage: "Authorization denied for action '" + action + "' on target '" + target + "'",
			wantTarget:  target,
		},
		{
			name:        "unauthorized without diagnostic reason",
			err:         &DeniedError{Code: v1.CodeInvalidAuthenticationInfo, Action: action, Target: target},
			wantStatus:  http.StatusUnauthorized,
			wantCode:    v1.CodeInvalidAuthenticationInfo,
			wantMessage: "Authorization denied for action '" + action + "' on target '" + target + "'",
			wantTarget:  target,
		},
		{
			name:        "unknown code",
			err:         &DeniedError{Code: "Unknown", Action: action, Target: target},
			wantStatus:  http.StatusInternalServerError,
			wantCode:    "Unknown",
			wantMessage: "Authorization denied for action '" + action + "' on target '" + target + "'",
			wantTarget:  target,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body, err := tt.err.ErrorResponse()
			require.NoError(t, err)
			require.NotNil(t, body.Error)
			require.Equal(t, tt.wantCode, body.Error.Code)
			require.Equal(t, tt.wantMessage, body.Error.Message)
			require.Equal(t, tt.wantTarget, body.Error.Target)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/", nil)
			response, err := tt.err.Response()
			require.NoError(t, err)
			require.NoError(t, response.Apply(t.Context(), w, req))

			require.Equal(t, tt.wantStatus, w.Code)
			require.Equal(t, "application/json", w.Header().Get("Content-Type"))

			var got v1.ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			require.Equal(t, body, got)
		})
	}
}

func Test_DeniedError_ResponseOmitsPrivateReason(t *testing.T) {
	t.Parallel()

	const identity = "alice@example.com"
	const privateTarget = "/planes/radius/local/resourceGroups/private/providers/Applications.Core/containers/secret"
	decision := Decision{
		Code:   v1.CodeOperationNotAssigned,
		Reason: "operation belongs to " + identity + " on " + privateTarget,
		Action: "Applications.Core/containers/write",
		Target: "/planes/radius/local/resourceGroups/public/providers/Applications.Core/containers/requested",
	}
	var denied *DeniedError
	require.ErrorAs(t, Apply(t.Context(), ModeEnforce, decision), &denied)
	require.Equal(t, decision.Reason, denied.Reason)

	response, err := denied.Response()
	require.NoError(t, err)
	w := httptest.NewRecorder()
	require.NoError(t, response.Apply(t.Context(), w, httptest.NewRequest(http.MethodPut, "/", nil)))
	require.Equal(t, http.StatusForbidden, w.Code)
	require.NotContains(t, w.Body.String(), identity)
	require.NotContains(t, w.Body.String(), privateTarget)
	require.NotContains(t, w.Body.String(), decision.Reason)

	var body v1.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, decision.Code, body.Error.Code)
	require.Equal(t, decision.Target, body.Error.Target)
	require.Equal(t, "Authorization denied for action '"+decision.Action+"' on target '"+decision.Target+"'", body.Error.Message)
}

func Test_DeniedError_ResponseRequiresRequestContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		action string
		target string
	}{
		{name: "both missing"},
		{name: "action missing", target: "/requested"},
		{name: "target missing", action: "read"},
		{name: "blank action", action: " \t\n", target: "/requested"},
		{name: "blank target", action: "read", target: " \t\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			denied := &DeniedError{
				Code:   v1.CodeAuthorizationFailed,
				Reason: "private diagnostic",
				Action: tt.action,
				Target: tt.target,
			}
			body, err := denied.ErrorResponse()
			require.EqualError(t, err, "authorization response requires an action and target")
			require.Equal(t, v1.ErrorResponse{}, body)
			response, err := denied.Response()
			require.EqualError(t, err, "authorization response requires an action and target")
			require.Nil(t, response)
		})
	}
}
