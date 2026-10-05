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

	deny := Decision{Allowed: false, Code: "GrantScopeExceeded", Reason: "target is outside the record"}
	allow := Decision{Allowed: true}

	tests := []struct {
		name     string
		mode     Mode
		decision Decision
		wantCode string
		wantErr  bool
		wantLog  bool
	}{
		{name: "unset mode allows denial", mode: "", decision: deny},
		{name: "off allows denial without logging", mode: ModeOff, decision: deny},
		{name: "off allows allowed", mode: ModeOff, decision: allow},
		{name: "dryRun logs denial", mode: ModeDryRun, decision: deny, wantLog: true},
		{name: "dryRun allows allowed without logging", mode: ModeDryRun, decision: allow},
		{name: "enforce rejects denial", mode: ModeEnforce, decision: deny, wantErr: true, wantCode: "GrantScopeExceeded"},
		{name: "enforce allows allowed", mode: ModeEnforce, decision: allow},
		{name: "unknown mode fails closed", mode: Mode("audit"), decision: deny, wantErr: true},
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

			if !tt.wantLog {
				require.Zero(t, logs.Len())
				return
			}

			entries := logs.All()
			require.Len(t, entries, 1)
			fields := entries[0].ContextMap()
			require.Equal(t, true, fields[LogFieldWouldDeny])
			require.Equal(t, tt.decision.Code, fields[LogFieldCode])
			require.Equal(t, tt.decision.Reason, fields[LogFieldReason])
		})
	}
}

func Test_DeniedError_Error(t *testing.T) {
	t.Parallel()

	err := &DeniedError{Code: "AuthorizationFailed", Reason: "no permission"}
	require.Equal(t, "authorization denied (AuthorizationFailed): no permission", err.Error())
}
