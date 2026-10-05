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
			wantMessage: "Authorization denied for action '" + action + "' on target '" + target + "': target is outside the record",
			wantTarget:  target,
		},
		{
			name:        "conflict",
			err:         &DeniedError{Code: v1.CodeOperationInputMismatch, Reason: "input hash does not match", Action: action, Target: target},
			wantStatus:  http.StatusConflict,
			wantCode:    v1.CodeOperationInputMismatch,
			wantMessage: "Authorization denied for action '" + action + "' on target '" + target + "': input hash does not match",
			wantTarget:  target,
		},
		{
			name:        "unavailable",
			err:         &DeniedError{Code: v1.CodeAuthorizationUnavailable, Reason: "permission store unreachable", Action: action, Target: target},
			wantStatus:  http.StatusServiceUnavailable,
			wantCode:    v1.CodeAuthorizationUnavailable,
			wantMessage: "Authorization denied for action '" + action + "' on target '" + target + "': permission store unreachable",
			wantTarget:  target,
		},
		{
			name:        "empty code defaults to AuthorizationFailed",
			err:         &DeniedError{Reason: "no permission", Action: action, Target: target},
			wantStatus:  http.StatusForbidden,
			wantCode:    v1.CodeAuthorizationFailed,
			wantMessage: "Authorization denied for action '" + action + "' on target '" + target + "': no permission",
			wantTarget:  target,
		},
		{
			name:        "no action or target",
			err:         &DeniedError{Code: v1.CodeAuthorizationFailed, Reason: "no permission"},
			wantStatus:  http.StatusForbidden,
			wantCode:    v1.CodeAuthorizationFailed,
			wantMessage: "Authorization denied: no permission",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body := tt.err.ErrorResponse()
			require.NotNil(t, body.Error)
			require.Equal(t, tt.wantCode, body.Error.Code)
			require.Equal(t, tt.wantMessage, body.Error.Message)
			require.Equal(t, tt.wantTarget, body.Error.Target)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/", nil)
			require.NoError(t, tt.err.Response().Apply(t.Context(), w, req))

			require.Equal(t, tt.wantStatus, w.Code)
			require.Equal(t, "application/json", w.Header().Get("Content-Type"))

			var got v1.ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			require.Equal(t, body, got)
		})
	}
}
