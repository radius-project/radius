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
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func Test_Apply(t *testing.T) {
	t.Parallel()

	deny := Decision{
		Allowed: false,
		Code:    "GrantScopeExceeded",
		Reason:  "target is outside the record",
		Action:  "Applications.Core/containers/write",
		Target:  "/planes/radius/local/resourceGroups/test/providers/Applications.Core/applications/app/containers/web",
		Rule:    "grant-scope",
	}
	allow := Decision{Allowed: true}
	noCode := Decision{
		Allowed: false,
		Reason:  "missing code",
		Action:  "Applications.Core/applications/read",
		Target:  "/planes/radius/local/resourceGroups/test",
		Rule:    "default-code",
	}

	tests := []struct {
		name            string
		mode            Mode
		decision        Decision
		wantCode        string
		wantErr         bool
		wantLogMessage  string
		wantLogField    string
		wantAbsentField string
	}{
		{name: "unset mode allows denial", mode: "", decision: deny},
		{name: "off allows denial without logging", mode: ModeOff, decision: deny},
		{name: "off allows allowed", mode: ModeOff, decision: allow},
		{
			name:            "dryRun logs denial",
			mode:            ModeDryRun,
			decision:        deny,
			wantLogMessage:  "authorization check would deny request",
			wantLogField:    LogFieldWouldDeny,
			wantAbsentField: LogFieldDenied,
		},
		{name: "dryRun allows allowed without logging", mode: ModeDryRun, decision: allow},
		{
			name:            "enforce rejects denial and logs denial",
			mode:            ModeEnforce,
			decision:        deny,
			wantErr:         true,
			wantCode:        "GrantScopeExceeded",
			wantLogMessage:  "authorization check denied request",
			wantLogField:    LogFieldDenied,
			wantAbsentField: LogFieldWouldDeny,
		},
		{name: "enforce allows allowed", mode: ModeEnforce, decision: allow},
		{name: "unknown mode fails closed", mode: Mode("audit"), decision: deny, wantErr: true},
		{name: "unknown mode fails closed for allowed", mode: Mode("audit"), decision: allow, wantErr: true},
		{
			name:            "dryRun logs default code when code is empty",
			mode:            ModeDryRun,
			decision:        noCode,
			wantCode:        CodeAuthorizationFailed,
			wantLogMessage:  "authorization check would deny request",
			wantLogField:    LogFieldWouldDeny,
			wantAbsentField: LogFieldDenied,
		},
		{
			name:            "enforce uses default code when code is empty",
			mode:            ModeEnforce,
			decision:        noCode,
			wantErr:         true,
			wantCode:        CodeAuthorizationFailed,
			wantLogMessage:  "authorization check denied request",
			wantLogField:    LogFieldDenied,
			wantAbsentField: LogFieldWouldDeny,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			core, logs := observer.New(zap.DebugLevel)
			ctx := logr.NewContext(t.Context(), zapr.NewLogger(zap.New(core)))

			err := Apply(ctx, tt.mode, tt.decision)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantCode != "" {
					var denied *DeniedError
					require.True(t, errors.As(err, &denied))
					require.Equal(t, tt.wantCode, denied.Code)
					require.Equal(t, tt.decision.Reason, denied.Reason)
				}
			} else {
				require.NoError(t, err)
			}

			if tt.wantLogMessage == "" {
				require.Zero(t, logs.Len())
				return
			}

			entries := logs.All()
			require.Len(t, entries, 1)
			require.Equal(t, tt.wantLogMessage, entries[0].Message)
			fields := entries[0].ContextMap()
			require.Equal(t, true, fields[tt.wantLogField])
			require.NotContains(t, fields, tt.wantAbsentField)
			wantCode := tt.decision.Code
			if tt.wantCode != "" {
				wantCode = tt.wantCode
			}
			require.Equal(t, wantCode, fields[LogFieldCode])
			require.Equal(t, tt.decision.Reason, fields[LogFieldReason])
			require.Equal(t, tt.decision.Action, fields[LogFieldAction])
			require.Equal(t, tt.decision.Target, fields[LogFieldTarget])
			require.Equal(t, tt.decision.Rule, fields[LogFieldRule])
		})
	}
}

func Test_DeniedError_Error(t *testing.T) {
	t.Parallel()

	err := &DeniedError{
		Code:   "AuthorizationFailed",
		Reason: "no permission",
		Action: "Applications.Core/containers/write",
		Target: "/planes/radius/local/resourceGroups/test/providers/Applications.Core/applications/app/containers/web",
	}
	require.Equal(t, `authorization denied (AuthorizationFailed) for action "Applications.Core/containers/write" on target "/planes/radius/local/resourceGroups/test/providers/Applications.Core/applications/app/containers/web"`, err.Error())
}

func Test_DeniedError_ErrorOmitsReason(t *testing.T) {
	t.Parallel()

	err := &DeniedError{
		Code:   "",
		Reason: "alice@example.com cannot access /planes/radius/local/resourceGroups/private-rg/providers/Applications.Core/applications/secret-app",
		Action: "Applications.Core/applications/read",
		Target: "/planes/radius/local/resourceGroups/public-rg/providers/Applications.Core/applications/public-app",
	}

	message := err.Error()
	require.Equal(t, `authorization denied (AuthorizationFailed) for action "Applications.Core/applications/read" on target "/planes/radius/local/resourceGroups/public-rg/providers/Applications.Core/applications/public-app"`, message)
	require.NotContains(t, message, "alice@example.com")
	require.NotContains(t, message, "private-rg")
	require.NotContains(t, message, "secret-app")
}
