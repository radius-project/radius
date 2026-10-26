/*
Copyright 2023 The Radius Authors.

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

package rpcheck

import (
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/authz/records"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
)

const target = "/planes/radius/local/resourceGroups/dev/providers/Applications.Core/applications/app"

func TestCheckOperationAndClaim(t *testing.T) {
	SetSelfForTest("applications-rp")
	ctx := t.Context()
	db := inmemory.NewClient()
	store := records.NewStore(db)
	body := []byte(`{"properties":{"environment":"dev"}}`)
	action := records.ActionFor("PUT", "Applications.Core/applications")

	rec := store.NewRecord(records.Record{Subject: "user:alice", Submitter: "ucp", Actions: []string{"*"}, Targets: []string{"/planes/radius/local/resourceGroups/dev"}})
	_, err := store.SaveRecord(ctx, rec, "")
	require.NoError(t, err)
	op := &records.Operation{RecordID: rec.ID, ResourceID: target, Action: action, AssignedComponent: "applications-rp", InputHash: records.InputHash(body)}
	require.NoError(t, store.CreateOperation(ctx, op))
	other := &records.Operation{RecordID: rec.ID, ResourceID: target, Action: action, AssignedComponent: "dynamic-rp", InputHash: records.InputHash(body)}
	require.NoError(t, store.CreateOperation(ctx, other))

	tests := []struct {
		name   string
		opID   string
		target string
		body   string
		code   string
	}{
		{"allowed", op.ID, target, string(body), ""},
		{"whitespace does not matter", op.ID, target, "{ \"properties\": {\"environment\": \"dev\"} }", ""},
		{"no operation", "", target, string(body), v1.CodeOperationNotAssigned},
		{"assigned elsewhere", other.ID, target, string(body), v1.CodeOperationNotAssigned},
		{"other resource", op.ID, target + "x", string(body), v1.CodeGrantScopeExceeded},
		{"tampered body", op.ID, target, `{"properties":{"environment":"prod"}}`, v1.CodeOperationInputMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, err := CheckOperation(ctx, store, tt.opID, tt.target, action, []byte(tt.body))
			require.NoError(t, err)
			require.Equal(t, tt.code, code)
		})
	}

	code, err := Claim(ctx, db, op.ID)
	require.NoError(t, err)
	require.Empty(t, code)

	code, err = Claim(ctx, db, other.ID)
	require.NoError(t, err)
	require.Equal(t, v1.CodeOperationNotAssigned, code)

	Finish(ctx, db, op.ID, true)
	got, _, err := store.GetOperation(ctx, op.ID)
	require.NoError(t, err)
	require.Equal(t, records.OperationRunning, got.Status, "finish is a no-op when the mode is off")

	require.NoError(t, store.CloseRecord(ctx, rec.ID, records.RecordClosed))
	code, err = CheckOperation(ctx, store, op.ID, target, action, body)
	require.NoError(t, err)
	require.Equal(t, v1.CodeExecutionRecordNotActive, code)
}

func TestClaimWithoutOperation(t *testing.T) {
	db := inmemory.NewClient()

	SetSelfForTest("ucp")
	code, err := Claim(t.Context(), db, "")
	require.NoError(t, err)
	require.Empty(t, code, "UCP's own async work needs no operation")

	SetSelfForTest("applications-rp")
	code, err = Claim(t.Context(), db, "")
	require.NoError(t, err)
	require.Equal(t, v1.CodeOperationNotAssigned, code)
}

func TestUnboundOperation(t *testing.T) {
	SetSelfForTest("applications-rp")
	ctx := t.Context()
	db := inmemory.NewClient()
	store := records.NewStore(db)
	body := []byte(`{"properties":{}}`)
	action := records.ActionFor("PUT", "Applications.Core/applications")

	ids, err := unboundOperation(ctx, store, target, action, body)
	require.NoError(t, err)

	code, err := CheckOperation(ctx, store, ids.OperationID, target, action, body)
	require.NoError(t, err)
	require.Empty(t, code, "later checks accept the unbound operation")

	code, err = Claim(ctx, db, ids.OperationID)
	require.NoError(t, err)
	require.Empty(t, code)
}
